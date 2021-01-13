package discoverkit

import (
	"context"
	"fmt"

	"github.com/dogmatiq/configkit"
	"github.com/dogmatiq/interopspec/discoverspec"
	"github.com/dogmatiq/linger/backoff"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Application is a Dogma application that was discovered on a Target.
type Application struct {
	// Identity is the application's identity.
	Identity configkit.Identity

	// Connection is the connection that was used to discover the application.
	Connection Connection
}

// ApplicationObserver is an interface for handling the discovery of Dogma
// applications.
type ApplicationObserver interface {
	// ApplicationDiscovered is called when a new application becomes available.
	//
	// ctx is canceled if the application becomes unavailable while
	// ApplicationDiscovered() is still executing.
	//
	// Note that it is possible for a single application identity to be
	// available via several connections.
	ApplicationDiscovered(ctx context.Context, a Application) error
}

// ApplicationObserverError indicates that an application discoverer was stopped
// because an ApplicationObserver produced an error.
type ApplicationObserverError struct {
	Discoverer  *ApplicationDiscoverer
	Observer    ApplicationObserver
	Application Application
	Cause       error
}

func (e ApplicationObserverError) Unwrap() error {
	return e.Cause
}

func (e ApplicationObserverError) Error() string {
	return fmt.Sprintf(
		"failure observing '%s' application: %s",
		e.Application.Identity,
		e.Cause,
	)
}

// ApplicationDiscoverer is an service for discovers Dogma applications running
// on gRPC targets.
//
// It implements ConnectObserver and forwards to an ApplicationObserver.
//
// It discovers applications on gRPC targets that implement the DiscoverAPI as
// defined in github.com/dogmatiq/interopspec/discoverspec. An implementation of
// this API is provided by the discoverkit.Server type.
type ApplicationDiscoverer struct {
	// Observer is the observer that is invoked when an application becomes
	// available.
	Observer ApplicationObserver

	// BackoffStrategy is the strategy that determines when to retry watching a
	// gRPC target for application availability changes.
	BackoffStrategy backoff.Strategy

	// LogError is an optional function that logs errors that occur while
	// attempting to watch a gRPC target.
	LogError func(Connection, error)
}

// TargetConnected is called when a new connection is established.
//
// ctx is canceled if the target becomes unavailable while TargetConnected() is
// still executing.
//
// The connection is automatically closed when TargetConnected() returns.
//
// It returns nil if the gRPC target does not implement the DiscoverAPI.
func (d *ApplicationDiscoverer) TargetConnected(ctx context.Context, c Connection) error {
	ctr := &backoff.Counter{
		Strategy: d.BackoffStrategy,
	}

	for {
		// Attempt to discover applications via the given connection.
		err := d.watch(ctx, c, ctr)

		// If the error is nil it means that the target does not implement the
		// DiscoverAPI. This is not an error, it simply means that we will never
		// discover any application on this target.
		if err == nil {
			return nil
		}

		// If the observer caused the failure we report that.
		if _, ok := err.(ApplicationObserverError); ok {
			return err
		}

		if d.LogError != nil {
			// If the parent context has been canceled we don't really want to
			// log about that, so we just bail early.
			if ctx.Err() != nil {
				return ctx.Err()
			}

			d.LogError(c, err)
		}

		// Finally, we sleep using the backoff counter until it's time to try
		// watching again.
		if err := ctr.Sleep(ctx, err); err != nil {
			return err
		}
	}
}

var emptyWatchApplicationsRequest discoverspec.WatchApplicationsRequest

// watch starts watching the server for announcements about application
// availability.
func (d *ApplicationDiscoverer) watch(
	ctx context.Context,
	c Connection,
	ctr *backoff.Counter,
) error {
	// Create a cancellable context specifically to abort the gRPC stream when
	// this watch attempt is completed.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream, err := discoverspec.
		NewDiscoverAPIClient(c).
		WatchApplications(ctx, &emptyWatchApplicationsRequest)
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

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		// Read the stream within the same group as the observer goroutines.
		// This ensures both that Wait() always has something to wait for, so
		// that it doesn't just return immediately, and that the whole group is
		// shutdown if the reading process itself fails.
		return d.recv(ctx, stream, c, g)
	})

	<-ctx.Done()

	return g.Wait()
}

// recv waits for the next message on the stream and starts/stops
// application-specific observer goroutines as necessary.
func (d *ApplicationDiscoverer) recv(
	ctx context.Context,
	stream discoverspec.DiscoverAPI_WatchApplicationsClient,
	c Connection,
	g *errgroup.Group,
) error {
	known := map[configkit.Identity]context.CancelFunc{}

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
				d.LogError(c, err)
			}

			continue
		}

		cancel, available := known[id]

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
			delete(known, id)
			continue
		}

		// Create a context specific for this application so that we can cancel it
		// if we receive an unavailable announcement just for this application.
		appCtx, cancel := context.WithCancel(ctx)
		known[id] = cancel

		// Start a new goroutine for the application.
		g.Go(func() error {
			defer cancel()
			return applicationDiscovered(
				appCtx,
				d,
				d.Observer,
				Application{
					Identity:   id,
					Connection: c,
				},
			)
		})
	}
}

// applicationDiscovered calls o.ApplicationDiscovered().
//
// If o.ApplicationDiscovered() returns a non-nil error it returns an
// ApplicationObserverError.
//
// If o.ApplicationDiscovered() returns a context.Canceled error *and* ctx is
// canceled, it returns nil.
func applicationDiscovered(
	ctx context.Context,
	d *ApplicationDiscoverer,
	o ApplicationObserver,
	a Application,
) error {
	err := o.ApplicationDiscovered(ctx, a)

	if err == nil {
		return nil
	}

	if err == context.Canceled && ctx.Err() == context.Canceled {
		return nil
	}

	return ApplicationObserverError{
		Discoverer:  d,
		Observer:    o,
		Application: a,
		Cause:       err,
	}
}
