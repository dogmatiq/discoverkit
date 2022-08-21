package discoverkit_test

import (
	"context"
	"time"

	. "github.com/dogmatiq/discoverkit"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("type StaticTargetDiscoverer", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		disc   StaticTargetDiscoverer
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 1*time.Second)

		disc = StaticTargetDiscoverer{
			{Name: "<target-1>"},
			{Name: "<target-2>"},
		}
	})

	AfterEach(func() {
		cancel()
	})

	Describe("func DiscoverTargets()", func() {
		It("invokes the observer immediately", func() {
			var targets []Target

			err := disc.DiscoverTargets(
				ctx,
				func(
					c context.Context,
					t Target,
				) {
					Expect(c).To(BeIdenticalTo(ctx))
					targets = append(targets, t)

					if len(targets) == len(disc) {
						cancel()
					}
				},
			)

			Expect(err).To(Equal(context.Canceled))
			Expect(targets).To(ConsistOf(disc))
		})
	})
})
