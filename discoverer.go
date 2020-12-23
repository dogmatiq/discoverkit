package discoverkit

import (
	"context"
)

// Discoverer is an interface for services that discover gRPC Targets.
type Discoverer interface {
	// Discover notifies o of targets that are discovered or undiscovered
	// until ctx is canceled or an error occurs.
	Discover(ctx context.Context, o TargetObserver) error
}

// TargetObserver is notified when a target is discovered or undiscovered.
type TargetObserver interface {
	// TargetDiscovered is called when a discoverer becomes aware of a target.
	//
	// The target is not necessarily online, or if it is online, not necessarily
	// available to serve requests.
	TargetDiscovered(*Target)

	// TargetUndiscovered is called when a previously discovered target is no
	// longer considered to exist.
	//
	// t must have previously been passed to TargetDiscovered. That is, the
	// pointer address itself.
	TargetUndiscovered(*Target)
}
