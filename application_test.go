package discoverkit_test

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"time"

	"github.com/dogmatiq/configkit"
	. "github.com/dogmatiq/discoverkit"
	"github.com/dogmatiq/interopspec/discoverspec"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
)

var _ = Describe("type ApplicationObserverError", func() {
	Describe("func Error()", func() {
		It("provides context about the application", func() {
			err := ApplicationObserverError{
				Application: Application{
					Identity: configkit.MustNewIdentity("<app-name>", "<app-key>"),
				},
				Cause: errors.New("<error>"),
			}

			Expect(err.Error()).To(Equal("failure observing '<app-name>/<app-key>' application: <error>"))
		})
	})

	Describe("func Unwrap()", func() {
		It("unwraps the causal error", func() {
			cause := errors.New("<error>")
			err := ApplicationObserverError{
				Cause: cause,
			}

			Expect(errors.Is(err, cause)).To(BeTrue())
		})
	})
})

var _ = Describe("type ApplicationDiscoverer", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc

		server    *serverStub
		listener  net.Listener
		gserver   *grpc.Server
		target    Target
		gconn     *grpc.ClientConn
		conn      Connection
		responses chan *discoverspec.WatchApplicationsResponse

		obs        *applicationObserverStub
		discoverer *ApplicationDiscoverer
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 200*time.Millisecond)

		responses = make(chan *discoverspec.WatchApplicationsResponse)

		server = &serverStub{
			WatchApplicationsFunc: func(
				_ *discoverspec.WatchApplicationsRequest,
				stream discoverspec.DiscoverAPI_WatchApplicationsServer,
			) error {
				for {
					select {
					case <-stream.Context().Done():
						return nil
					case r, ok := <-responses:
						if !ok {
							return nil
						}

						if err := stream.Send(r); err != nil {
							return err
						}
					}
				}
			},
		}

		var err error
		listener, err = net.Listen("tcp", ":")
		Expect(err).ShouldNot(HaveOccurred())

		gserver = grpc.NewServer()

		target = Target{
			Name: listener.Addr().String(),
			DialOptions: []grpc.DialOption{
				grpc.WithInsecure(),
			},
		}

		gconn, err = grpc.Dial(target.Name, target.DialOptions...)
		Expect(err).ShouldNot(HaveOccurred())

		conn = Connection{
			ClientConnInterface: gconn,
			Target:              target,
		}

		obs = &applicationObserverStub{}

		discoverer = &ApplicationDiscoverer{
			Observer: obs,
		}
	})

	AfterEach(func() {
		cancel()

		if gconn != nil {
			gconn.Close()
		}

		if gserver != nil {
			gserver.Stop()
		}

		if listener != nil {
			listener.Close()
		}
	})

	Describe("func TargetConnected()", func() {
		When("the server implements the discovery API", func() {
			BeforeEach(func() {
				discoverspec.RegisterDiscoverAPIServer(gserver, server)
				go gserver.Serve(listener)
			})

			When("an app becomes available", func() {
				BeforeEach(func() {
					go func() {
						res := &discoverspec.WatchApplicationsResponse{
							Identity: &discoverspec.Identity{
								Name: "<app-name>",
								Key:  "<app-key>",
							},
							Available: true,
						}

						select {
						case <-ctx.Done():
						case responses <- res:
						}
					}()
				})

				It("invokes the observer", func() {
					obs.ApplicationDiscoveredFunc = func(
						_ context.Context,
						app Application,
					) error {
						defer cancel()
						defer GinkgoRecover()

						Expect(app).To(Equal(
							Application{
								Identity:   configkit.MustNewIdentity("<app-name>", "<app-key>"),
								Connection: conn,
							},
						))

						return nil
					}

					err := discoverer.TargetConnected(ctx, conn)
					Expect(err).To(Equal(context.Canceled))
				})

				It("returns an error when the observer returns an error", func() {
					cause := errors.New("<error>")

					obs.ApplicationDiscoveredFunc = func(
						context.Context,
						Application,
					) error {
						return cause
					}

					err := discoverer.TargetConnected(ctx, conn)
					Expect(err).To(Equal(
						ApplicationObserverError{
							Discoverer: discoverer,
							Observer:   obs,
							Application: Application{
								Identity:   configkit.MustNewIdentity("<app-name>", "<app-key>"),
								Connection: conn,
							},
							Cause: cause,
						},
					))
				})

				It("cancels the observer context when the server goes offline", func() {
					obs.ApplicationDiscoveredFunc = func(
						ctx context.Context,
						a Application,
					) error {
						// Stop the gRPC server.
						gserver.Stop()

						// Then wait for the application-specific context to be
						// done, either because it's canceled properly, or
						// because the test times out.
						<-ctx.Done()

						// Then we cancel the test's context. If everything is
						// behaving correctly this should happen BEFORE the test
						// times out, so we see a context.Canceled error and not
						// DeadlineExceeded.
						cancel()

						return ctx.Err()
					}

					err := discoverer.TargetConnected(ctx, conn)
					Expect(err).To(Equal(context.Canceled))
				})

				It("cancels the observer context when the application becomes unavailable", func() {
					obs.ApplicationDiscoveredFunc = func(
						ctx context.Context,
						app Application,
					) error {
						// Write the "unavailable" response.
						res := &discoverspec.WatchApplicationsResponse{
							Identity: &discoverspec.Identity{
								Name: app.Identity.Name,
								Key:  app.Identity.Key,
							},
							Available: false,
						}

						select {
						case <-ctx.Done():
						case responses <- res:
						}

						// Then wait for the application-specific context to be
						// done, either because it's canceled properly, or
						// because the test times out.
						<-ctx.Done()

						// Then we cancel the test's context. If everything is
						// behaving correctly this should happen BEFORE the test
						// times out, so we see a context.Canceled error and not
						// DeadlineExceeded.
						cancel()

						return ctx.Err()
					}

					err := discoverer.TargetConnected(ctx, conn)
					Expect(err).To(Equal(context.Canceled))
				})

				It("cancels the observer context when the server ends the stream", func() {
					obs.ApplicationDiscoveredFunc = func(
						ctx context.Context,
						app Application,
					) error {
						// Close the "responses" channel which causes the server
						// to return from the WatchApplications() method.
						close(responses)

						// Then wait for the application-specific context to be
						// done, either because it's canceled properly, or
						// because the test times out.
						<-ctx.Done()

						// Then we cancel the test's context. If everything is
						// behaving correctly this should happen BEFORE the test
						// times out, so we see a context.Canceled error and not
						// DeadlineExceeded.
						cancel()

						return ctx.Err()
					}

					err := discoverer.TargetConnected(ctx, conn)
					Expect(err).To(Equal(context.Canceled))
				})

				It("does not invoke the observer if the server sends a duplicate response", func() {
					var count uint32

					obs.ApplicationDiscoveredFunc = func(
						_ context.Context,
						app Application,
					) error {
						// Send the duplicate response the first time
						// ApplicationDiscovered() is called.
						if atomic.AddUint32(&count, 1) == 1 {
							res := &discoverspec.WatchApplicationsResponse{
								Identity: &discoverspec.Identity{
									Name: "<app-name>",
									Key:  "<app-key>",
								},
								Available: true,
							}

							select {
							case <-ctx.Done():
								return ctx.Err()
							case responses <- res:
								return nil
							}
						}

						// If we've already been called return an error.
						return errors.New("unexpected call")
					}

					// There's not much we can "expect" here other than that the
					// test times out, since we're testing that something
					// DOESN'T happen in its own goroutine.
					err := discoverer.TargetConnected(ctx, conn)
					Expect(err).To(Equal(context.DeadlineExceeded))
				})
			})

			When("the server sends an invalid identity", func() {
				BeforeEach(func() {
					go func() {
						res := &discoverspec.WatchApplicationsResponse{
							Identity:  &discoverspec.Identity{}, // note: empty identity is invalid
							Available: true,
						}

						select {
						case <-ctx.Done():
						case responses <- res:
						}
					}()
				})

				It("does not invoke the observer", func() {
					obs.ApplicationDiscoveredFunc = func(
						context.Context,
						Application,
					) error {
						return errors.New("unexpected call")
					}

					err := discoverer.TargetConnected(ctx, conn)
					Expect(err).To(Equal(context.DeadlineExceeded))
				})

				It("logs the error", func() {
					discoverer.LogError = func(
						c Connection,
						err error,
					) {
						defer GinkgoRecover()
						defer cancel()

						Expect(c).To(Equal(conn))
						Expect(err).To(MatchError(`invalid application identity: invalid name "", names must be non-empty, printable UTF-8 strings with no whitespace`))
					}

					err := discoverer.TargetConnected(ctx, conn)
					Expect(err).To(Equal(context.Canceled))
				})
			})

			When("the server produces an unexpected error", func() {
				BeforeEach(func() {
					server.WatchApplicationsFunc = func(
						_ *discoverspec.WatchApplicationsRequest,
						stream discoverspec.DiscoverAPI_WatchApplicationsServer,
					) error {
						return errors.New("<error>")
					}
				})

				It("logs the error", func() {
					discoverer.LogError = func(
						c Connection,
						err error,
					) {
						defer GinkgoRecover()
						defer cancel()

						Expect(c).To(Equal(conn))
						Expect(err).To(MatchError(`unable to read from stream: rpc error: code = Unknown desc = <error>`))
					}

					err := discoverer.TargetConnected(ctx, conn)
					Expect(err).To(Equal(context.Canceled))
				})
			})
		})

		When("the server does not implement the discovery API", func() {
			BeforeEach(func() {
				go gserver.Serve(listener)
			})

			It("returns nil immediately", func() {
				err := discoverer.TargetConnected(ctx, conn)
				Expect(err).ShouldNot(HaveOccurred())
			})
		})
	})
})

// applicationObserverStub is a test implementation of the ApplicationObserver
// interface.
type applicationObserverStub struct {
	ApplicationDiscoveredFunc func(context.Context, Application) error
}

// ApplicationDiscovered calls o.ApplicationDiscoveredFunc(ctx, a) if it is non-nil.
func (o *applicationObserverStub) ApplicationDiscovered(ctx context.Context, a Application) error {
	if o.ApplicationDiscoveredFunc != nil {
		return o.ApplicationDiscoveredFunc(ctx, a)
	}

	return nil
}

type serverStub struct {
	WatchApplicationsFunc func(*discoverspec.WatchApplicationsRequest, discoverspec.DiscoverAPI_WatchApplicationsServer) error
}

func (s *serverStub) WatchApplications(
	req *discoverspec.WatchApplicationsRequest,
	stream discoverspec.DiscoverAPI_WatchApplicationsServer,
) error {
	if s.WatchApplicationsFunc != nil {
		return s.WatchApplicationsFunc(req, stream)
	}

	return nil
}
