package discoverkit

import (
	"context"

	"golang.org/x/sync/errgroup"
)

// StaticDiscoverer is a Discoverer that always "discovers" a fixed set of
// pre-configured targets.
type StaticDiscoverer []Target

// Discover invokes o.TargetDiscovered() when a new target is discovered.
//
// Each invocation is made on its own goroutine. The context passed to
// o.TargetDiscovered() is canceled when the target becomes unavailable, or the
// discoverer itself is stopped due to cancelation of ctx.
//
// The discoverer stops and returns a TargetObserverError if any call to
// o.TargetDiscovered() returns a non-nil error.
func (d StaticDiscoverer) Discover(ctx context.Context, o TargetObserver) error {
	g, ctx := errgroup.WithContext(ctx)

	for _, t := range d {
		t := t // capture loop variable

		g.Go(func() error {
			return targetDiscovered(ctx, d, o, t)
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	// All of the observer calls have returned successfully so there's nothing
	// left to do except wait until ctx is canceled.
	<-ctx.Done()
	return ctx.Err()
}
