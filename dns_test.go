package discoverkit_test

import (
	"context"
	"errors"
	"net"
	"time"

	. "github.com/dogmatiq/discoverkit"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("type DNSDiscoverer", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		obs    *targetObserverStub
		res    *dnsResolverStub
		disc   *DNSDiscoverer
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 1*time.Second)
		ctx, cancel = context.WithCancel(ctx)

		obs = &targetObserverStub{}

		res = &dnsResolverStub{}

		disc = &DNSDiscoverer{
			QueryHost: "<query-host>",
			Resolver:  res,
		}
	})

	AfterEach(func() {
		cancel()
	})

	Describe("func Discover()", func() {
		It("notifies the observer of discovery when addresses are found", func() {
			res.LookupHostFunc = func(_ context.Context, host string) ([]string, error) {
				cancel()

				Expect(host).To(Equal("<query-host>"))
				return []string{"<addr-1>", "<addr-2>"}, nil
			}

			var targets []Target
			obs.TargetDiscoveredFunc = func(t DiscoveredTarget) {
				Expect(t.ID).To(BeNumerically(">", 0))
				Expect(t.Discoverer).To(Equal(disc))

				targets = append(targets, t.Target)
			}

			err := disc.Discover(ctx, obs)
			Expect(err).To(Equal(context.Canceled))
			Expect(targets).To(ConsistOf(
				Target{
					Name: "<addr-1>",
				},
				Target{
					Name: "<addr-2>",
				},
			))
		})

		It("notifies the observer of undiscovery when addresses go away", func() {
			disc.QueryInterval = 10 * time.Millisecond

			res.LookupHostFunc = func(context.Context, string) ([]string, error) {
				// Replace this function on the stub so that it returns a subset
				// of the addresses that it returned the first time.
				res.LookupHostFunc = func(context.Context, string) ([]string, error) {
					return []string{"<addr-2>"}, nil
				}

				return []string{"<addr-1>", "<addr-2>"}, nil
			}

			obs.TargetUndiscoveredFunc = func(t DiscoveredTarget) {
				Expect(t.Target).To(Equal(
					Target{
						Name: "<addr-1>",
					},
				))

				// Remove this function from the stub to prevent a failure when
				// <addr-2> becomes unavailable when the discover is stopped.
				obs.TargetUndiscoveredFunc = nil

				cancel()
			}

			err := disc.Discover(ctx, obs)
			Expect(err).To(Equal(context.Canceled))
		})

		It("notifies the observer of undiscovery when the discoverer is stopped", func() {
			disc.QueryInterval = 10 * time.Millisecond

			res.LookupHostFunc = func(context.Context, string) ([]string, error) {
				cancel()
				return []string{"<addr-1>", "<addr-2>"}, nil
			}

			targets := map[uint64]DiscoveredTarget{}

			obs.TargetDiscoveredFunc = func(t DiscoveredTarget) {
				targets[t.ID] = t
			}

			obs.TargetUndiscoveredFunc = func(t DiscoveredTarget) {
				x := targets[t.ID]
				Expect(t).To(Equal(x))
				delete(targets, t.ID)
			}

			err := disc.Discover(ctx, obs)
			Expect(err).To(Equal(context.Canceled))
			Expect(targets).To(BeEmpty())
		})

		It("uses net.DefaultResolver by default", func() {
			disc.QueryHost = "localhost"
			disc.Resolver = nil

			obs.TargetDiscoveredFunc = func(t DiscoveredTarget) {
				cancel()

				Expect(t.Name).To(
					Or(
						Equal("127.0.0.1"),
						Equal("::1"),
					),
				)
			}

			err := disc.Discover(ctx, obs)
			Expect(err).To(Equal(context.Canceled))
		})

		When("there is a NewTargets() function", func() {
			It("uses the targets returned by NewTargets()", func() {
				disc.NewTargets = func(_ context.Context, addr string) ([]Target, error) {
					return []Target{
						{Name: addr + "-A"},
						{Name: addr + "-B"},
					}, nil
				}

				res.LookupHostFunc = func(context.Context, string) ([]string, error) {
					cancel()
					return []string{"<addr-1>", "<addr-2>"}, nil
				}

				var targets []Target
				obs.TargetDiscoveredFunc = func(t DiscoveredTarget) {
					targets = append(targets, t.Target)
				}

				err := disc.Discover(ctx, obs)
				Expect(err).To(Equal(context.Canceled))
				Expect(targets).To(ConsistOf(
					Target{
						Name: "<addr-1>-A",
					},
					Target{
						Name: "<addr-1>-B",
					},
					Target{
						Name: "<addr-2>-A",
					},
					Target{
						Name: "<addr-2>-B",
					},
				))
			})

			It("properly tracks addresses for which NewTargets() returns an empty slice", func() {
				disc.QueryInterval = 10 * time.Millisecond

				disc.NewTargets = func(context.Context, string) ([]Target, error) {
					// Replace this function on the stub to ensure that it
					// doesn't get called again for the same address.
					disc.NewTargets = func(context.Context, string) ([]Target, error) {
						Fail("unexpected second invocation of NewTargets()")
						return nil, nil
					}

					return nil, nil
				}

				res.LookupHostFunc = func(context.Context, string) ([]string, error) {
					// Replace this function on the stub to cancel the context
					// the *second* time that it is called.
					res.LookupHostFunc = func(context.Context, string) ([]string, error) {
						cancel()

						// Return the same address on this second invocation,
						// ensuring that NewTargets() would be called again if
						// addresses with no associated targets were not tracked
						// correctly.
						return []string{"<addr>"}, nil
					}

					return []string{"<addr>"}, nil
				}

				obs.TargetDiscoveredFunc = func(t DiscoveredTarget) {
					Fail("unexpoected notification of discovered target")
				}

				err := disc.Discover(ctx, obs)
				Expect(err).To(Equal(context.Canceled))
			})

			It("returns an error if NewTargets() returns an error", func() {
				disc.NewTargets = func(context.Context, string) ([]Target, error) {
					return nil, errors.New("<error>")
				}

				res.LookupHostFunc = func(context.Context, string) ([]string, error) {
					return []string{"<addr>"}, nil
				}

				err := disc.Discover(ctx, obs)
				Expect(err).To(MatchError("<error>"))
			})
		})

		When("the resolver fails", func() {
			It("does not propagate not-found errors", func() {
				res.LookupHostFunc = func(context.Context, string) ([]string, error) {
					cancel()
					return nil, &net.DNSError{
						IsNotFound: true,
					}
				}

				err := disc.Discover(ctx, obs)
				Expect(err).To(Equal(context.Canceled)) // note: not the net.DNSError
			})

			It("propagates other errors", func() {
				res.LookupHostFunc = func(context.Context, string) ([]string, error) {
					return nil, errors.New("<error>")
				}

				err := disc.Discover(ctx, obs)
				Expect(err).To(MatchError("<error>"))
			})
		})
	})
})

// dnsResolverStub is a test implementation of the DNSResolver interface.
type dnsResolverStub struct {
	LookupHostFunc func(ctx context.Context, host string) ([]string, error)
}

// LookupHost calls r.LookHostFunc(ctx, host) if it is non-nil.
func (r *dnsResolverStub) LookupHost(ctx context.Context, host string) ([]string, error) {
	if r.LookupHostFunc == nil {
		return nil, &net.DNSError{
			IsNotFound: true,
		}
	}

	return r.LookupHostFunc(ctx, host)
}
