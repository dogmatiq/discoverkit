package discoverkit_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
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
		It("invokes the observer when a target is discovered", func() {
			res.LookupHostFunc = func(_ context.Context, host string) ([]string, error) {
				cancel()

				Expect(host).To(Equal("<query-host>"))
				return []string{"<addr-1>", "<addr-2>"}, nil
			}

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

				return nil
			}

			err := disc.Discover(ctx, obs)
			Expect(err).To(Equal(context.Canceled))
			Expect(targets).To(ConsistOf(
				Target{Name: "<addr-1>"},
				Target{Name: "<addr-2>"},
			))
		})

		It("cancels the observer context when a target goes away", func() {
			disc.QueryInterval = 10 * time.Millisecond

			res.LookupHostFunc = func(context.Context, string) ([]string, error) {
				// Replace this function on the stub so that it returns a subset
				// of the addresses that it returned the first time.
				res.LookupHostFunc = func(context.Context, string) ([]string, error) {
					return []string{"<addr-2>"}, nil
				}

				return []string{"<addr-1>", "<addr-2>"}, nil
			}

			ok := make(chan struct{})

			obs.TargetDiscoveredFunc = func(
				targetCtx context.Context,
				t Target,
			) error {
				<-targetCtx.Done()

				// We expect "<addr-1>" to become unavailable based on the setup
				// of res.LookupHostFunc above.
				if t.Name == "<addr-1>" {
					close(ok)
					return nil
				}

				// Otherwise we would expect only to be canceled when the
				// discover is stopped by cancelling the parent context.
				if ctx.Err() == nil {
					return fmt.Errorf("unexpected cancelation for %s", t.Name)
				}

				return nil
			}

			result := make(chan error, 1)
			go func() {
				result <- disc.Discover(ctx, obs)
			}()

			select {
			case <-ok:
				// We expect the "ok" channel to be closed when the context
				// associated with "<addr-1>" is canceled.
				cancel() // stop the discoverer

			case err := <-result:
				// We don't expect the discoverer to stop before "ok" is closed.
				Expect(err).ShouldNot(HaveOccurred(), "discoverer stopped prematurely")

			case <-ctx.Done():
				// We don't expect the parent context to be canceled before "ok"
				// is closed.
				Expect(ctx.Err()).ShouldNot(HaveOccurred(), "context canceled prematurely")
			}

			// Now that we called cancel() we expect the discoverer to stop in a
			// timely manner.
			err := <-result
			Expect(err).To(Equal(context.Canceled))
		})

		It("cancels the context passed to the observer when the discoverer is stopped", func() {
			disc.QueryInterval = 10 * time.Millisecond

			res.LookupHostFunc = func(context.Context, string) ([]string, error) {
				cancel()
				return []string{"<addr-1>", "<addr-2>"}, nil
			}

			var running int32 // atomic

			obs.TargetDiscoveredFunc = func(
				ctx context.Context,
				_ Target,
			) error {
				n := atomic.AddInt32(&running, 1)
				defer atomic.AddInt32(&running, -1)

				if int(n) == 2 {
					go cancel()
				}

				<-ctx.Done()
				return ctx.Err()
			}

			err := disc.Discover(ctx, obs)
			Expect(err).To(Equal(context.Canceled))
			Expect(atomic.LoadInt32(&running)).To(BeZero())
		})

		It("uses net.DefaultResolver by default", func() {
			disc.QueryHost = "localhost"
			disc.Resolver = nil

			obs.TargetDiscoveredFunc = func(
				_ context.Context,
				t Target,
			) error {
				cancel()

				Expect(t.Name).To(
					Or(
						Equal("127.0.0.1"),
						Equal("::1"),
					),
				)

				return nil
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

					return nil
				}

				err := disc.Discover(ctx, obs)
				Expect(err).To(Equal(context.Canceled))
				Expect(targets).To(ConsistOf(
					Target{Name: "<addr-1>-A"},
					Target{Name: "<addr-1>-B"},
					Target{Name: "<addr-2>-A"},
					Target{Name: "<addr-2>-B"},
				))
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

				obs.TargetDiscoveredFunc = func(
					context.Context,
					Target,
				) error {
					return errors.New("unexpected call to TargetDiscovered()")
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

		When("the resolver returns an error", func() {
			It("ignores not-found errors", func() {
				res.LookupHostFunc = func(context.Context, string) ([]string, error) {
					cancel()
					return nil, &net.DNSError{
						IsNotFound: true,
					}
				}

				err := disc.Discover(ctx, obs)
				Expect(err).To(Equal(context.Canceled)) // note: not the net.DNSError
			})

			It("returns other errors", func() {
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
