package discoverkit_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/dogmatiq/discoverkit"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("type StaticTargetDiscoverer", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		obs    *targetObserverStub
		disc   StaticTargetDiscoverer
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 1*time.Second)
		ctx, cancel = context.WithCancel(ctx)

		obs = &targetObserverStub{}

		disc = StaticTargetDiscoverer{
			{Name: "<target-1>"},
			{Name: "<target-2>"},
		}
	})

	AfterEach(func() {
		cancel()
	})

	Describe("func Discover()", func() {
		It("invokes the observer immediately", func() {
			var (
				m       sync.Mutex
				targets []Target
			)

			obs.TargetDiscoveredFunc = func(
				_ context.Context,
				t Target,
			) error {
				m.Lock()
				targets = append(targets, t)
				m.Unlock()

				if len(targets) == len(disc) {
					cancel()
				}

				return nil
			}

			err := disc.DiscoverTargets(ctx, obs)
			Expect(err).To(Equal(context.Canceled))
			Expect(targets).To(ConsistOf(disc))
		})

		It("cancels the context passed to the observer when the discoverer is stopped", func() {
			var running int32 // atomic

			obs.TargetDiscoveredFunc = func(
				ctx context.Context,
				_ Target,
			) error {
				n := atomic.AddInt32(&running, 1)
				defer atomic.AddInt32(&running, -1)

				if int(n) == len(disc) {
					go cancel()
				}

				<-ctx.Done()
				return ctx.Err()
			}

			err := disc.DiscoverTargets(ctx, obs)
			Expect(err).To(Equal(context.Canceled))
			Expect(atomic.LoadInt32(&running)).To(BeZero())
		})

		It("stops the discoverer if the observer produces an error", func() {
			obs.TargetDiscoveredFunc = func(
				_ context.Context,
				t Target,
			) error {
				if t.Name == "<target-2>" {
					return errors.New("<error>")
				}

				return nil
			}

			err := disc.DiscoverTargets(ctx, obs)
			Expect(err).To(Equal(TargetObserverError{
				Discoverer: disc,
				Observer:   obs,
				Target:     Target{Name: "<target-2>"},
				Cause:      errors.New("<error>"),
			}))
		})
	})
})
