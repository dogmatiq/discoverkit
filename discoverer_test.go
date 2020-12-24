package discoverkit_test

import (
	"context"
	"errors"

	. "github.com/dogmatiq/discoverkit"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("type DiscoverObserverError", func() {
	Describe("func Error()", func() {
		It("provides context about the target", func() {
			err := DiscoverObserverError{
				Target: Target{Name: "<target>"},
				Cause:  errors.New("<error>"),
			}

			Expect(err.Error()).To(Equal("failure observing '<target>' target: <error>"))
		})
	})

	Describe("func Unwrap()", func() {
		It("unwraps the causal error", func() {
			cause := errors.New("<error>")
			err := DiscoverObserverError{
				Cause: cause,
			}

			Expect(errors.Is(err, cause)).To(BeTrue())
		})
	})
})

// discoverObserverStub is a test implementation of the DiscoverObserver
// interface.
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
