package discoverkit

import "context"

// StaticDiscoverer is a Discoverer that always "discovers" a fixed set of
// pre-configured targets.
type StaticDiscoverer []*Target

// Discover notifies o of targets that are discovered or undiscovered until ctx
// is canceled or an error occurs.
func (d StaticDiscoverer) Discover(ctx context.Context, o TargetObserver) error {
	for _, t := range d {
		o.TargetDiscovered(t)
		defer o.TargetUndiscovered(t)
	}

	<-ctx.Done()
	return ctx.Err()
}
