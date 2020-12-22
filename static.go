package discoverkit

import "context"

// StaticDiscoverer is a Discoverer that always "discovers" a fixed set of of
// pre-configured targets.
type StaticDiscoverer struct {
	Targets []Target
}

// Discover notifies o of targets that are discovered (or "undiscovered")
// until ctx is canceled or an error occurs.
func (d *StaticDiscoverer) Discover(ctx context.Context, o TargetObserver) error {
	for _, t := range d.Targets {
		dt := DiscoveredTarget{
			Target:     t,
			ID:         DiscoveredTargetID(),
			Discoverer: d,
		}

		o.TargetDiscovered(dt)
		defer o.TargetUndiscovered(dt)
	}

	<-ctx.Done()
	return ctx.Err()
}
