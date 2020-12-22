package discoverkit

import (
	"context"
	"sync/atomic"
)

// Discoverer is an interface for services that discover gRPC Targets.
type Discoverer interface {
	// Discover notifies o of targets that are discovered or undiscovered
	// until ctx is canceled or an error occurs.
	Discover(ctx context.Context, o TargetObserver) error
}

// DiscoveredTarget is a Target that was discovered by a discoverer.
type DiscoveredTarget struct {
	Target

	// ID is a unique identifier for the target.
	// It must be unique within the current process.
	ID uint64

	// Discoverer is the discoverer that located this target.
	Discoverer Discoverer
}

var nextID uint64

// DiscoveredTargetID allocates a new unique ID for a DiscoveredTarget.
func DiscoveredTargetID() uint64 {
	return atomic.AddUint64(&nextID, 1)
}

// TargetObserver is notified when a target is discovered or undiscovered.
type TargetObserver interface {
	// TargetDiscovered is called when a discoverer becomes aware of a target.
	//
	// The target is not necessarily online, or if it is online, not necessarily
	// available to serve requests.
	TargetDiscovered(DiscoveredTarget)

	// TargetUndiscovered is called when a previously discovered target is no
	// longer considered to exist.
	TargetUndiscovered(DiscoveredTarget)
}
