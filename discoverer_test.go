package discoverkit_test

import (
	"sync"

	. "github.com/dogmatiq/discoverkit"
)

// targetObserverStub is a test implementation of the TargetObserver interface.
type targetObserverStub struct {
	m                      sync.Mutex
	TargetDiscoveredFunc   func(DiscoveredTarget)
	TargetUndiscoveredFunc func(DiscoveredTarget)
}

// TargetDiscovered calls o.TargetDiscoveredFunc(t) if it is non-nil.
func (o *targetObserverStub) TargetDiscovered(t DiscoveredTarget) {
	if o.TargetDiscoveredFunc != nil {
		o.m.Lock()
		defer o.m.Unlock()
		o.TargetDiscoveredFunc(t)
	}
}

// TargetUndiscovered calls o.TargetUndiscoveredFunc(t) if it is non-nil.
func (o *targetObserverStub) TargetUndiscovered(t DiscoveredTarget) {
	if o.TargetUndiscoveredFunc != nil {
		o.m.Lock()
		defer o.m.Unlock()
		o.TargetUndiscoveredFunc(t)
	}
}
