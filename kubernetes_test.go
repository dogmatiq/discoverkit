package discoverkit_test

import (
	"context"
	"os"
	"time"

	. "github.com/dogmatiq/discoverkit"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("type KubernetesEnvironmentTargetDiscoverer", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		disc   *KubernetesEnvironmentTargetDiscoverer
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 1*time.Second)

		disc = &KubernetesEnvironmentTargetDiscoverer{}

		environment := map[string]string{
			"DISCOVERKIT1_SERVICE_HOST":        "app1.example.org",
			"DISCOVERKIT1_SERVICE_PORT_CUSTOM": "12345",
			"DISCOVERKIT1_SERVICE_PORT_DOGMA":  "50555",

			"DISCOVERKIT2_SERVICE_HOST":       "app2.example.org",
			"DISCOVERKIT2_SERVICE_PORT_DOGMA": "80888",

			"DISCOVERKIT3_SERVICE_PORT_DOGMA": "", // empty ports should be ignored

			"DISCOVERKIT4_SERVICE_HOST":       "", // empty hosts should be ignored
			"DISCOVERKIT4_SERVICE_PORT_DOGMA": "50555",
		}

		for k, v := range environment {
			os.Setenv(k, v)
		}

		DeferCleanup(func() {
			for k := range environment {
				os.Unsetenv(k)
			}
		})
	})

	AfterEach(func() {
		cancel()
	})

	Describe("func DiscoverTargets()", func() {
		It("invokes the observer for each service with the named port", func() {
			var (
				actual []Target
				expect = []Target{
					{Name: "app1.example.org:50555"},
					{Name: "app2.example.org:80888"},
				}
			)

			err := disc.DiscoverTargets(
				ctx,
				func(
					c context.Context,
					t Target,
				) {
					Expect(c).To(BeIdenticalTo(ctx))
					actual = append(actual, t)

					if len(actual) == len(expect) {
						cancel()
					}
				},
			)

			Expect(err).To(Equal(context.Canceled))
			Expect(actual).To(ConsistOf(expect))
		})

		It("allows use of a custom port name", func() {
			disc.PortName = "custom"

			var (
				actual []Target
				expect = []Target{
					{Name: "app1.example.org:12345"},
				}
			)

			err := disc.DiscoverTargets(
				ctx,
				func(
					c context.Context,
					t Target,
				) {
					Expect(c).To(BeIdenticalTo(ctx))
					actual = append(actual, t)

					if len(actual) == len(expect) {
						cancel()
					}
				},
			)

			Expect(err).To(Equal(context.Canceled))
			Expect(actual).To(ConsistOf(expect))
		})
	})
})
