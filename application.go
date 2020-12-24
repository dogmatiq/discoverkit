package discoverkit

import (
	"context"

	"github.com/dogmatiq/interopspec/discoverspec"
	"github.com/dogmatiq/linger/backoff"
	"go.uber.org/multierr"
	"golang.org/x/sync/errgroup"
)

// ApplicationDiscoverer discovers Dogma applications hosted by engines running
// on gRPC targets.
//
// It implements ConnectObserver and forwards to an ApplicationObserver.
//
// It requires the server to implement the DiscoverAPI service as defined in
// github.com/dogmatiq/interopspec/discoverspec. An implementation this service
// is provided by the github.com/dogmatiq/discoverkit/api package.
type ApplicationDiscoverer struct {
	Observer        ApplicationObserver
	BackoffStrategy backoff.Strategy
	IsFatal         func(context.Context, Connection, error) (bool, error)
}

var emptyWatchApplicationsRequest discoverspec.WatchApplicationsRequest

// TargetConnected is called when a new connection is established.
//
// ctx is canceled if the target becomes unavailable while TargetConnected()
// is still executing.
//
// The connection is automatically closed when TargetConnected() returns.
func (d *ApplicationDiscoverer) TargetConnected(ctx context.Context, c Connection) error {
	ctr := &backoff.Counter{
		Strategy: d.BackoffStrategy,
	}

	for {
		// Attempt to discover applications via the given connection.
		err := d.discover(ctx, c, ctr)

		// If a nil error was returned it means that the target does not
		// implement the DiscoverAPI service, so we will never discover any
		// applications.
		if err == nil {
			return nil
		}

		// Otherwise, if the context has been canceled we don't really care what
		// the actual error was, we just want to bail.
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// If there is an IsFatal hook present we ask it whether we should bail
		// due to the error that occurred.
		if d.IsFatal != nil {
			fatal, isFatalErr := d.IsFatal(ctx, c, err)
			if isFatalErr != nil {
				return multierr.Combine(err, isFatalErr)
			}

			if fatal {
				return err
			}
		}

		// Finally, we sleep using the backoff counter until it's time to try
		// discovery again.
		if err := ctr.Sleep(ctx, err); err != nil {
			return err
		}
	}
}

func (d *ApplicationDiscoverer) discover(
	ctx context.Context,
	c Connection,
	ctr *backoff.Counter,
) error {
	cli := discoverspec.NewDiscoverAPIClient(c)

	stream, err := cli.WatchApplications(ctx, &emptyWatchApplicationsRequest)
	if err != nil {
		return err
	}

	ctr.Reset()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return d.watch(ctx, g, c, stream)
	})

	return g.Wait()
}

func (d *ApplicationDiscoverer) watch(
	ctx context.Context,
	g *errgroup.Group,
	c Connection,
	stream discoverspec.DiscoverAPI_WatchApplicationsClient,
) error {
	known := map[string]context.CancelFunc{}

	for {
		res, err := stream.Recv()
		if err != nil {
			return err
		}

		if cancel, ok := known[res.ApplicationKey]; ok {
			if !res.Available {
				delete(known, res.ApplicationKey)
				cancel()
			}

			continue
		}

		// Create a new context specifically for this application. It will
		// be canceled if application becomes unavailable.
		appCtx, cancel := context.WithCancel(ctx)
		known[res.ApplicationKey] = cancel

		// Start a new goroutine for the application.
		g.Go(func() error {
			return d.Observer.ApplicationDiscovered(
				appCtx,
				Application{
					Key:        res.ApplicationKey,
					Connection: c,
				},
			)
		})
	}
}

// ApplicationObserver is an interface for handling the discovery of Dogma
// applications.
type ApplicationObserver interface {
	// ApplicationDiscovered is called when a new connection is established.
	//
	// ctx is canceled if the target becomes unavailable while
	// ApplicationDiscovered() is still executing.
	ApplicationDiscovered(ctx context.Context, a Application) error
}

// Application is a Dogma application that was discovered on a Target.
type Application struct {
	// Key is the application's unique key.
	Key string

	// Connection is the connection that was used to discover the applications.
	Connection Connection
}
