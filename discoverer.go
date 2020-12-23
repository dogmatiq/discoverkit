package discoverkit

import (
	"context"
	"fmt"
)

// Discoverer is an interface for services that discover gRPC Targets.
type Discoverer interface {
	// Discover invokes o.TargetDiscovered() when a new target is discovered.
	//
	// Each invocation is made on its own goroutine. The context passed to
	// o.TargetDiscovered() is canceled when the target is "undiscovered", or
	// the discoverer itself is stopped due to cancelation of ctx.
	//
	// The discoverer stops and returns a DiscoverObserverError if any call to
	// o.TargetDiscovered() returns a non-nil error.
	Discover(ctx context.Context, o DiscoverObserver) error
}

// DiscoverObserver is an interface for types that accept notifications about
// target discovery.
type DiscoverObserver interface {
	// TargetDiscovered is called when a new target is discovered.
	//
	// ctx is canceled if the target is undiscovered while TargetDiscovered() is
	// still executing.
	TargetDiscovered(ctx context.Context, t Target) error
}

// DiscoverObserverError indicates that a discoverer was stopped because a
// DiscoverObserver produced an error.
type DiscoverObserverError struct {
	Discoverer Discoverer
	Observer   DiscoverObserver
	Target     Target
	Cause      error
}

func (e DiscoverObserverError) Unwrap() error {
	return e.Cause
}

func (e DiscoverObserverError) Error() string {
	return fmt.Sprintf(
		"observing %s target: %s",
		e.Target.Name,
		e.Cause,
	)
}
