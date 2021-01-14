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

var _ = Describe("type DNSTargetDiscoverer", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		disc   *DNSTargetDiscoverer
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 1*time.Second)

		disc = &DNSTargetDiscoverer{
			QueryHost: "<query-host>",
			LookupHost: func(context.Context, string) ([]string, error) {
				return nil, &net.DNSError{
					IsNotFound: true,
				}
			},
		}
	})

	AfterEach(func() {
		cancel()
	})

	Describe("func DiscoverTargets()", func() {
		It("invokes the observer when a target is discovered", func() {
			disc.LookupHost = func(_ context.Context, host string) ([]string, error) {
				cancel()

				Expect(host).To(Equal("<query-host>"))
				return []string{"<addr-1>", "<addr-2>"}, nil
			}

			var targets []Target

			err := disc.DiscoverTargets(
				ctx,
				func(
					_ context.Context,
					t Target,
				) {
					targets = append(targets, t)
				},
			)

			Expect(err).To(Equal(context.Canceled))
			Expect(targets).To(ConsistOf(
				Target{Name: "<addr-1>"},
				Target{Name: "<addr-2>"},
			))
		})

		It("cancels the observer context when a target goes away", func() {
			disc.QueryInterval = 10 * time.Millisecond

			disc.LookupHost = func(context.Context, string) ([]string, error) {
				// Replace this function on the stub so that it returns a subset
				// of the addresses that it returned the first time.
				disc.LookupHost = func(context.Context, string) ([]string, error) {
					return []string{"<addr-2>"}, nil
				}

				return []string{"<addr-1>", "<addr-2>"}, nil
			}

			done := make(chan struct{})

			go disc.DiscoverTargets(
				ctx,
				func(
					targetCtx context.Context,
					t Target,
				) {
					if t.Name == "<addr-1>" {
						go func() {
							<-targetCtx.Done()
							close(done)
						}()
					}
				},
			)

			select {
			case <-done:
			case <-ctx.Done():
				Expect(ctx.Err()).ShouldNot(HaveOccurred())
			}
		})

		It("cancels the observer context when the discoverer is stopped", func() {
			discoverCtx, cancel := context.WithCancel(ctx)

			disc.LookupHost = func(context.Context, string) ([]string, error) {
				cancel()
				return []string{"<addr-1>", "<addr-2>"}, nil
			}

			done := make(chan struct{}, 2)

			err := disc.DiscoverTargets(
				discoverCtx,
				func(
					targetCtx context.Context,
					t Target,
				) {
					go func() {
						<-targetCtx.Done()
						done <- struct{}{}
					}()
				},
			)

			Expect(err).To(Equal(context.Canceled))

			select {
			case <-done:
			case <-ctx.Done():
				Expect(ctx.Err()).ShouldNot(HaveOccurred())
			}

			select {
			case <-done:
			case <-ctx.Done():
				Expect(ctx.Err()).ShouldNot(HaveOccurred())
			}
		})

		It("uses net.DefaultResolver.LookupHost() by default", func() {
			disc.QueryHost = "localhost"
			disc.LookupHost = nil

			err := disc.DiscoverTargets(
				ctx,
				func(
					_ context.Context,
					t Target,
				) {
					cancel()
					Expect(t.Name).To(
						Or(
							Equal("127.0.0.1"),
							Equal("::1"),
						),
					)
				},
			)

			Expect(err).To(Equal(context.Canceled))
		})

		When("there is a NewTargets() function", func() {
			It("passes the targets returned by NewTargets() to the observer", func() {
				disc.NewTargets = func(_ context.Context, addr string) ([]Target, error) {
					return []Target{
						{Name: addr + "-A"},
						{Name: addr + "-B"},
					}, nil
				}

				disc.LookupHost = func(context.Context, string) ([]string, error) {
					cancel()
					return []string{"<addr-1>", "<addr-2>"}, nil
				}

				var targets []Target

				err := disc.DiscoverTargets(
					ctx,
					func(
						_ context.Context,
						t Target,
					) {
						targets = append(targets, t)
					},
				)

				Expect(err).To(Equal(context.Canceled))
				Expect(targets).To(ConsistOf(
					Target{Name: "<addr-1>-A"},
					Target{Name: "<addr-1>-B"},
					Target{Name: "<addr-2>-A"},
					Target{Name: "<addr-2>-B"},
				))
			})

			It("allows NewTargets() to return an empty slice", func() {
				disc.QueryInterval = 10 * time.Millisecond

				disc.LookupHost = func(context.Context, string) ([]string, error) {
					cancel()
					return []string{"<addr>"}, nil
				}

				disc.NewTargets = func(context.Context, string) ([]Target, error) {
					return nil, nil
				}

				err := disc.DiscoverTargets(
					ctx,
					func(
						context.Context,
						Target,
					) {
						Fail("unexpected call")
					},
				)

				Expect(err).To(Equal(context.Canceled))
			})

			It("does not call NewTargets() again for an address that already produced zero targets", func() {
				disc.QueryInterval = 10 * time.Millisecond

				disc.NewTargets = func(context.Context, string) ([]Target, error) {
					// Replace this function on the stub to ensure that it
					// doesn't get called again for the same address.
					disc.NewTargets = func(context.Context, string) ([]Target, error) {
						return nil, errors.New("unexpected second invocation of NewTargets()")
					}

					return nil, nil
				}

				disc.LookupHost = func(context.Context, string) ([]string, error) {
					// Replace this function on the stub to cancel the context
					// the *second* time that it is called.
					disc.LookupHost = func(context.Context, string) ([]string, error) {
						cancel()

						// Return the same address on this second invocation,
						// ensuring that NewTargets() would be called again if
						// addresses with no associated targets were not tracked
						// correctly.
						return []string{"<addr>"}, nil
					}

					return []string{"<addr>"}, nil
				}

				err := disc.DiscoverTargets(ctx, nil)
				Expect(err).To(Equal(context.Canceled))
			})

			It("returns an error if NewTargets() returns an error", func() {
				disc.NewTargets = func(context.Context, string) ([]Target, error) {
					return nil, errors.New("<error>")
				}

				disc.LookupHost = func(context.Context, string) ([]string, error) {
					return []string{"<addr>"}, nil
				}

				err := disc.DiscoverTargets(ctx, nil)
				Expect(err).To(MatchError("<error>"))
			})
		})

		When("the resolver returns an error", func() {
			It("ignores not-found errors", func() {
				disc.LookupHost = func(context.Context, string) ([]string, error) {
					cancel()
					return nil, &net.DNSError{
						IsNotFound: true,
					}
				}

				err := disc.DiscoverTargets(ctx, nil)
				Expect(err).To(Equal(context.Canceled)) // note: not the net.DNSError
			})

			It("returns other errors", func() {
				disc.LookupHost = func(context.Context, string) ([]string, error) {
					return nil, errors.New("<error>")
				}

				err := disc.DiscoverTargets(ctx, nil)
				Expect(err).To(MatchError("<error>"))
			})
		})
	})
})
