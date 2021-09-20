package discoverkit

import (
	"context"
	"net"

	"github.com/dogmatiq/configkit"
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

// AdvertiseTarget advertises a target so that it may be discovered by a
// TargetDiscoverer.
//
// addr is the address on which the gRPC server accepts connections.
//
// This discovery method does not require advertising, nil is returned
// immediately.
func (d StaticTargetDiscoverer) AdvertiseTarget(
	ctx context.Context,
	addr net.Addr,
	applications []configkit.Identity,
) error {
	return nil
}
