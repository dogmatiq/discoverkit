package discoverkit_test

import (
	"context"
	"time"

	. "github.com/dogmatiq/discoverkit"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("type StaticDiscoverer", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		obs    *targetObserverStub
		disc   StaticDiscoverer
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 1*time.Second)
		ctx, cancel = context.WithCancel(ctx)

		obs = &targetObserverStub{}

		disc = StaticDiscoverer{
			{Name: "<target-1>"},
			{Name: "<target-2>"},
		}
	})

	AfterEach(func() {
		cancel()
	})

	Describe("func Discover()", func() {
		It("notifies the observer of discovery immediately", func() {
			var targets []*Target

			obs.TargetDiscoveredFunc = func(t *Target) {
				targets = append(targets, t)

				if len(targets) == len(disc) {
					obs.TargetUndiscoveredFunc = nil
					cancel()
				}
			}

			obs.TargetUndiscoveredFunc = func(*Target) {
				Fail("observer unexpectedly notified of target unavailability")
			}

			err := disc.Discover(ctx, obs)
			Expect(err).To(Equal(context.Canceled))
			Expect(targets).To(ConsistOf(disc))
		})

		It("notifies the observer of undiscovery when the discoverer is stopped", func() {
			targets := map[*Target]struct{}{}

			obs.TargetDiscoveredFunc = func(t *Target) {
				targets[t] = struct{}{}

				if len(targets) == len(disc) {
					cancel()
				}
			}

			obs.TargetUndiscoveredFunc = func(t *Target) {
				_, ok := targets[t]
				Expect(ok).To(BeTrue())
				delete(targets, t)
			}

			err := disc.Discover(ctx, obs)
			Expect(err).To(Equal(context.Canceled))
			Expect(targets).To(BeEmpty())
		})
	})
})
