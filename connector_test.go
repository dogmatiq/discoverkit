package discoverkit_test

import (
	"context"
	"errors"

	. "github.com/dogmatiq/discoverkit"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
)

var _ = Describe("type Connector", func() {
	var (
		target1, target2 Target
		obs              *connectObserverStub
		connector        *Connector
	)

	BeforeEach(func() {
		target1 = Target{
			Name: "<target-1>",
			DialOptions: []grpc.DialOption{
				grpc.WithInsecure(),
			},
		}

		target2 = Target{
			Name: "<target-2>",
			DialOptions: []grpc.DialOption{
				grpc.WithInsecure(),
			},
		}

		obs = &connectObserverStub{}

		connector = &Connector{
			Observer: obs,
		}
	})

	Describe("func TargetDiscovered()", func() {
		It("invokes the observer", func() {
			obs.TargetConnectedFunc = func(
				ctx context.Context,
				c Connection,
			) error {
				Expect(c.ClientConnInterface).NotTo(BeNil())
				Expect(c.Target).To(Equal(target1))
				return errors.New("<error>")
			}

			err := connector.TargetDiscovered(
				context.Background(),
				target1,
			)
			Expect(err).To(MatchError("<error>"))
		})

		When("there is an ignore predicate", func() {
			BeforeEach(func() {
				connector.Ignore = func(
					_ context.Context,
					t Target,
				) (bool, error) {
					return t.Name == target1.Name, nil
				}
			})

			It("invokes the observer if the target is not ignored", func() {
				obs.TargetConnectedFunc = func(
					context.Context,
					Connection,
				) error {
					return errors.New("<error>")
				}

				err := connector.TargetDiscovered(
					context.Background(),
					target2,
				)
				Expect(err).To(MatchError("<error>"))
			})

			It("does not invoke the observer if the target is ignored", func() {
				obs.TargetConnectedFunc = func(
					context.Context,
					Connection,
				) error {
					return errors.New("unexpected call")
				}

				err := connector.TargetDiscovered(
					context.Background(),
					target1,
				)
				Expect(err).ShouldNot(HaveOccurred())
			})

			It("returns the error produced by the ignore predicate", func() {
				connector.Ignore = func(
					_ context.Context,
					t Target,
				) (bool, error) {
					return false, errors.New("<error>")
				}

				err := connector.TargetDiscovered(
					context.Background(),
					target1,
				)
				Expect(err).To(MatchError("<error>"))
			})
		})

		When("the dialer returns an error", func() {
			BeforeEach(func() {
				connector.Dial = func(context.Context, string, ...grpc.DialOption) (*grpc.ClientConn, error) {
					return nil, errors.New("<error>")
				}
			})

			It("returns the error without invoking the observer", func() {
				obs.TargetConnectedFunc = func(
					context.Context,
					Connection,
				) error {
					Fail("unexpected call")
					return nil
				}

				err := connector.TargetDiscovered(
					context.Background(),
					target1,
				)
				Expect(err).To(MatchError("<error>"))
			})
		})
	})
})

// connectObserverStub is a test implementation of the ConnectObserver
// interface.
type connectObserverStub struct {
	TargetConnectedFunc func(context.Context, Connection) error
}

// TargetConnected calls o.TargetConnectedFunc(ctx, c) if it is non-nil.
func (o *connectObserverStub) TargetConnected(ctx context.Context, c Connection) error {
	if o.TargetConnectedFunc != nil {
		return o.TargetConnectedFunc(ctx, c)
	}

	return nil
}
