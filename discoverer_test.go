package discoverkit_test

import (
	"context"

	. "github.com/dogmatiq/discoverkit"
)

// discoverObserverStub is a test implementation of the TargetObserver interface.
type discoverObserverStub struct {
	TargetDiscoveredFunc func(context.Context, Target) error
}

// TargetDiscovered calls o.TargetDiscoveredFunc(ctx, t) if it is non-nil.
func (o *discoverObserverStub) TargetDiscovered(ctx context.Context, t Target) error {
	if o.TargetDiscoveredFunc != nil {
		return o.TargetDiscoveredFunc(ctx, t)
	}

	return nil
}
