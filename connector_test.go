package discoverkit_test

import (
	"errors"
	"sync"

	. "github.com/dogmatiq/discoverkit"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
)

var _ = Describe("type Connector", func() {
	var (
		target1, target2 *Target
		obs              *connectionObserverStub
		connector        *Connector
	)

	BeforeEach(func() {
		target1 = &Target{
			Name: "<target-1>",
			DialOptions: []grpc.DialOption{
				grpc.WithInsecure(),
			},
		}

		target2 = &Target{
			Name: "<target-2>",
			DialOptions: []grpc.DialOption{
				grpc.WithInsecure(),
			},
		}

		obs = &connectionObserverStub{}

		connector = &Connector{
			Observer: obs,
		}
	})

	Describe("func TargetDiscovered()", func() {
		It("notifies the observer of connection availability", func() {
			called := false
			obs.ConnectionAvailableFunc = func(
				t *Target,
				conn grpc.ClientConnInterface,
			) {
				Expect(t).To(Equal(target1))
				Expect(conn).NotTo(BeNil())
				called = true
			}

			connector.TargetDiscovered(target1)

			Expect(called).To(BeTrue())
		})

		It("does not notify the observer if the target has already been discovered", func() {
			connector.TargetDiscovered(target1)

			obs.ConnectionAvailableFunc = func(
				*Target,
				grpc.ClientConnInterface,
			) {
				Fail("unexpected call")
			}

			connector.TargetDiscovered(target1)
		})

		When("there is an ignore predicate", func() {
			BeforeEach(func() {
				connector.Ignore = func(t *Target) bool {
					return t == target1
				}
			})

			It("notifies the observer if the target is not ignored", func() {
				called := false
				obs.ConnectionAvailableFunc = func(
					t *Target,
					_ grpc.ClientConnInterface,
				) {
					Expect(t).To(Equal(target2))
					called = true
				}

				connector.TargetDiscovered(target2)

				Expect(called).To(BeTrue())
			})

			It("does not notify the observer if the target is ignored", func() {
				obs.ConnectionAvailableFunc = func(
					*Target,
					grpc.ClientConnInterface,
				) {
					Fail("unexpected call")
				}

				connector.TargetDiscovered(target1)
			})
		})

		When("dialing fails", func() {
			BeforeEach(func() {
				connector.Dial = func(string, ...grpc.DialOption) (*grpc.ClientConn, error) {
					return nil, errors.New("<error>")
				}
			})

			It("ignores the error", func() {
				obs.ConnectionAvailableFunc = func(
					*Target,
					grpc.ClientConnInterface,
				) {
					Fail("unexpected call")
				}

				connector.TargetDiscovered(target1)
			})

			It("invokes the OnDialError() function if it is present", func() {
				called := false
				connector.OnDialError = func(
					t *Target,
					err error,
				) {
					Expect(t).To(Equal(target1))
					Expect(err).To(MatchError("<error>"))
					called = true
				}

				connector.TargetDiscovered(target1)

				Expect(called).To(BeTrue())
			})
		})
	})

	Describe("func TargetUndiscovered()", func() {
		It("notifies the observer of connection unavailability", func() {
			connector.TargetDiscovered(target1)

			called := false
			obs.ConnectionUnavailableFunc = func(
				t *Target,
				conn grpc.ClientConnInterface,
			) {
				Expect(t).To(Equal(target1))
				Expect(conn).NotTo(BeNil())
				called = true
			}

			connector.TargetUndiscovered(target1)

			Expect(called).To(BeTrue())
		})
	})
})

// connectionObserverStub is a test implementation of the ConnectionObserver
// interface.
type connectionObserverStub struct {
	m                         sync.Mutex
	ConnectionAvailableFunc   func(*Target, grpc.ClientConnInterface)
	ConnectionUnavailableFunc func(*Target, grpc.ClientConnInterface)
}

// ConnectionAvailable calls o.ConnectionAvailableFunc(t,conn) if it is non-nil.
func (o *connectionObserverStub) ConnectionAvailable(t *Target, c grpc.ClientConnInterface) {
	if o.ConnectionAvailableFunc != nil {
		o.m.Lock()
		defer o.m.Unlock()
		o.ConnectionAvailableFunc(t, c)
	}
}

// ConnectionUnavailable calls o.ConnectionUnavailableFunc(t,conn) if it is non-nil.
func (o *connectionObserverStub) ConnectionUnavailable(t *Target, c grpc.ClientConnInterface) {
	if o.ConnectionUnavailableFunc != nil {
		o.m.Lock()
		defer o.m.Unlock()
		o.ConnectionUnavailableFunc(t, c)
	}
}
