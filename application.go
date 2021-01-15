package discoverkit

import (
	"context"
	"fmt"

	"github.com/dogmatiq/configkit"
	"github.com/dogmatiq/interopspec/discoverspec"
	"github.com/dogmatiq/linger/backoff"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Application is a Dogma application that was discovered on a Target.
type Application struct {
	// Identity is the application's identity.
	Identity configkit.Identity

	// Target is the gRPC target that is hosting the application.
	Target Target

	// Connection is the connection that was used to discover the application.
	Connection grpc.ClientConnInterface
}

// ApplicationObserver is a function that handles the discovery of a Dogma
// application running on a gRPC target.
//
// ctx is canceled when the application becomes unavailable or the discoverer is
// stopped. ctx is NOT canceled when the observer function returns and as such
// may be used by goroutines started by the observer.
//
// The discoverer MAY block on calls to the observer. It is the observer's
// responsibility to start new goroutines to handle background tasks, as
// appropriate.
//
// Note that it is possible for multiple targets to host the same application.
type ApplicationObserver func(ctx context.Context, a Application)

// Dialer is a function for connecting to gRPC targets.
//
// It matches the signature of grpc.DialContext().
type Dialer func(context.Context, string, ...grpc.DialOption) (*grpc.ClientConn, error)

// ApplicationDiscoverer is a service that discovers Dogma applications running
// on gRPC targets.
//
// It discovers applications on gRPC targets that implement the DiscoverAPI as
// defined in github.com/dogmatiq/interopspec/discoverspec. An implementation of
// this API is provided by the discoverkit.Server type.
type ApplicationDiscoverer struct {
	// Dial is the function used to dial gRPC targets.
	//
	// If it is nil, grpc.DialContext() is used.
	Dial Dialer

	// BackoffStrategy is the strategy that determines when to retry watching a
	// gRPC target for application availability changes.
	BackoffStrategy backoff.Strategy

	// LogError is an optional function that logs errors that occur while
	// attempting to watch a gRPC target.
	LogError func(Target, error)
}

// DiscoverApplications invokes an observer for each Dogma application target
// that is discovered on a specific gRPC target.
//
// It returns a nil error if the target is contactable but it does not implement
// the DiscoverAPI service. Otherwise, it runs until ctx is canceled.
//
// Errors that occur while communicating with the target are logged to the
// LogError function,if present, before retrying. The retry interval is
// determined by the discoverer's BackoffStrategy.
//
// The context passed to the observer is canceled when the application becomes
// unavailable or the discover is stopped.
//
// The discoverer MAY block on calls to the observer. It is the observer's
// responsibility to start new goroutines to handle background tasks, as
// appropriate.
func (d *ApplicationDiscoverer) DiscoverApplications(
	ctx context.Context,
	t Target,
	obs ApplicationObserver,
) error {
	ctr := &backoff.Counter{
		Strategy: d.BackoffStrategy,
	}

	for {
		// Attempt to discover applications via the given connection.
		err := d.watch(ctx, ctr, t, obs)

		// If the error is nil it means that the target does not implement the
		// DiscoverAPI. This is not an error, it simply means that we will never
		// discover any application on this target.
		if err == nil {
			return nil
		}

		// If the parent context has been canceled we don't really care what
		// happens. Bail here before we log it.
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Log the error, if a log function was provided.
		if d.LogError != nil {
			d.LogError(t, err)
		}

		// Finally, we sleep using the backoff counter until it's time to try
		// watching again.
		if err := ctr.Sleep(ctx, err); err != nil {
			return err
		}
	}
}

var emptyWatchApplicationsRequest discoverspec.WatchApplicationsRequest

// watch dials a target and watches it for updates to application
// availability.
func (d *ApplicationDiscoverer) watch(
	ctx context.Context,
	ctr *backoff.Counter,
	t Target,
	obs ApplicationObserver,
) error {
	// Dial the target using the configurered dialer, or otherwise using the
	// default gRPC dialer.
	dial := d.Dial
	if dial == nil {
		dial = grpc.DialContext
	}

	conn, err := dial(ctx, t.Name, t.DialOptions...)
	if err != nil {
		return fmt.Errorf("unable to dial target: %w", err)
	}
	defer conn.Close()

	// Create a cancellable context specifically to abort the gRPC stream when
	// this function returns. There's no Close() method on a stream, it's
	// lifetime is tied to the context that created it.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cli := discoverspec.NewDiscoverAPIClient(conn)
	stream, err := cli.WatchApplications(ctx, &emptyWatchApplicationsRequest)
	if err != nil {
		// Note that the gRPC package does NOT report "unimplemented" errors
		// here, even though this is where we call the RPC. Instead, they are
		// delivered on the first call to stream.Recv().
		return fmt.Errorf("unable to watch target: %w", err)
	}

	// Reset the backoff counter now that we've made a successful call to the
	// server. We expect at this point that the emptyWatchApplicationsRequest
	// has at least been sent successfully, even though we don't see its result
	// until we call stream.Recv().
	ctr.Reset()

	return d.recv(ctx, t, conn, stream, obs)
}

// recv waits for the next response on the "watch stream" and invokes observers
// / cancels their contexts as applications become available and unavailable.
func (d *ApplicationDiscoverer) recv(
	ctx context.Context,
	t Target,
	conn *grpc.ClientConn,
	stream discoverspec.DiscoverAPI_WatchApplicationsClient,
	obs ApplicationObserver,
) error {
	applications := map[configkit.Identity]context.CancelFunc{}

	defer func() {
		for _, cancel := range applications {
			cancel()
		}
	}()

	for {
		res, err := stream.Recv()
		if err != nil {
			// If the error indicates that the DiscoverAPI has not been
			// implemented we return without an error to indicate there's
			// nothing more to be done.
			if st, ok := status.FromError(err); ok {
				if st.Code() == codes.Unimplemented {
					return nil
				}
			}

			return fmt.Errorf("unable to read from stream: %w", err)
		}

		id, err := configkit.NewIdentity(
			res.GetIdentity().GetName(),
			res.GetIdentity().GetKey(),
		)
		if err != nil {
			// The server has sent an invalid application identity. We log about
			// it if necessary but otherwise ignore the error.
			//
			// This approach is taken (as opposed to returning the error) so
			// that we can continue to use other applications with well-formed
			// identities on the same server.
			if d.LogError != nil {
				err = fmt.Errorf("invalid application identity: %w", err)
				d.LogError(t, err)
			}

			continue
		}

		cancel, available := applications[id]

		if res.Available == available {
			// There has been no change in availability. Perhaps the server
			// re-announced the same application. This is not the *expected*
			// behavior, but we are lenient in the interest of robustness.
			continue
		}

		if !res.Available {
			// The application has been marked as unavailable. Cancel its
			// goroutine and remove it from the list of known applications.
			cancel()
			delete(applications, id)
			continue
		}

		// Create a context specific for this application. It will be canceled
		// if the server sends an "unavailable" response for this application
		// over the stream.
		appCtx, cancel := context.WithCancel(ctx)
		applications[id] = cancel

		obs(appCtx, Application{
			Identity:   id,
			Target:     t,
			Connection: conn,
		})
	}
}
