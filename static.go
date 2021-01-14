package discoverkit

import (
	"context"
)

// StaticTargetDiscoverer is a TargetDiscoverer that always "discovers" a fixed
// set of pre-configured targets.
type StaticTargetDiscoverer []Target

// DiscoverTargets invokes an observer for each gRPC target that is discovered.
//
// It runs until ctx is canceled or an error occurs.
//
// The context passed to the observer is canceled when the target becomes
// unavailable or the discover is stopped.
//
// The discoverer MAY block on calls to the observer. It is the observer's
// responsibility to start new goroutines to handle background tasks, as
// appropriate.
func (d StaticTargetDiscoverer) DiscoverTargets(ctx context.Context, obs TargetObserver) error {
	for _, t := range d {
		obs(ctx, t)
	}

	<-ctx.Done()

	return ctx.Err()
}
