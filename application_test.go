package discoverkit_test

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/dogmatiq/configkit"
	. "github.com/dogmatiq/discoverkit"
	"github.com/dogmatiq/interopspec/discoverspec"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const appKey = "420a8fe8-0c57-44e0-8332-d5c5f93a63fc"

var _ = Describe("type ApplicationDiscoverer", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc

		server    *serverStub
		listener  net.Listener
		gserver   *grpc.Server
		target    Target
		responses chan *discoverspec.WatchApplicationsResponse

		discoverer *ApplicationDiscoverer
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 200*time.Millisecond)
		DeferCleanup(cancel)

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
		DeferCleanup(func() {
			listener.Close()
		})

		gserver = grpc.NewServer()
		DeferCleanup(func() {
			gserver.GracefulStop()
		})

		target = Target{
			Name: listener.Addr().String(),
			DialOptions: []grpc.DialOption{
				grpc.WithTransportCredentials(
					insecure.NewCredentials(),
				),
			},
		}

		discoverer = &ApplicationDiscoverer{}
	})

	Describe("func DiscoverApplications()", func() {
		When("the server implements the discovery API", func() {
			BeforeEach(func() {
				discoverspec.RegisterDiscoverAPIServer(gserver, server)

				done := make(chan struct{})
				go func() {
					defer close(done)
					gserver.Serve(listener)
				}()

				DeferCleanup(func() {
					listener.Close()
					<-done
				})
			})

			When("an application becomes available", func() {
				BeforeEach(func() {
					done := make(chan struct{})
					go func() {
						defer close(done)
						res := &discoverspec.WatchApplicationsResponse{
							Identity: &discoverspec.Identity{
								Name: "<app-name>",
								Key:  appKey,
							},
							Available: true,
						}

						select {
						case <-ctx.Done():
						case responses <- res:
						}
					}()

					DeferCleanup(func() {
						<-done
					})
				})

				It("invokes the observer", func() {
					err := discoverer.DiscoverApplications(
						ctx,
						target,
						func(
							_ context.Context,
							app Application,
						) {
							defer cancel()

							Expect(app).To(MatchAllFields(
								Fields{
									"Identity":   Equal(configkit.MustNewIdentity("<app-name>", appKey)),
									"Target":     Equal(target),
									"Connection": Not(BeNil()),
								},
							))
						},
					)

					Expect(err).To(Equal(context.Canceled))
				})

				It("cancels the observer context when the server goes offline", func() {
					canceled := make(chan struct{})
					done := make(chan struct{})
					defer func() {
						<-done
					}()

					go func() {
						defer close(done)

						discoverer.DiscoverApplications(
							ctx,
							target,
							func(
								appCtx context.Context,
								_ Application,
							) {
								gserver.Stop()

								go func() {
									<-appCtx.Done()
									close(canceled)
								}()
							},
						)
					}()

					select {
					case <-canceled:
					case <-ctx.Done():
						Expect(ctx.Err()).ShouldNot(HaveOccurred())
					}
				})

				It("cancels the observer context when the application becomes unavailable", func() {
					done := make(chan struct{})
					defer func() {
						<-done
					}()

					canceled := make(chan struct{})

					go func() {
						defer close(done)

						discoverer.DiscoverApplications(
							ctx,
							target,
							func(
								appCtx context.Context,
								app Application,
							) {
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

								go func() {
									<-appCtx.Done()
									close(canceled)
								}()
							},
						)
					}()

					select {
					case <-canceled:
					case <-ctx.Done():
						Expect(ctx.Err()).ShouldNot(HaveOccurred())
					}
				})

				It("cancels the observer context when the server ends the stream", func() {
					done := make(chan struct{})
					defer func() {
						<-done
					}()

					canceled := make(chan struct{})

					go func() {
						defer close(done)

						discoverer.DiscoverApplications(
							ctx,
							target,
							func(
								appCtx context.Context,
								app Application,
							) {
								// Close the "responses" channel which causes the
								// server to return from the WatchApplications()
								// method.
								close(responses)

								go func() {
									<-appCtx.Done()
									close(canceled)
								}()
							},
						)
					}()

					select {
					case <-canceled:
					case <-ctx.Done():
						Expect(ctx.Err()).ShouldNot(HaveOccurred())
					}
				})

				It("does not invoke the observer if the server sends a duplicate response", func() {
					count := 0

					err := discoverer.DiscoverApplications(
						ctx,
						target,
						func(
							appCtx context.Context,
							app Application,
						) {
							if count == 0 {
								// Send the duplicate response the first time
								// ApplicationDiscovered() is called.
								go func() {
									res := &discoverspec.WatchApplicationsResponse{
										Identity: &discoverspec.Identity{
											Name: "<app-name>",
											Key:  appKey,
										},
										Available: true,
									}

									select {
									case <-ctx.Done():
									case responses <- res:
									}
								}()
							}

							count++
						},
					)

					// There's not much we can "expect" here other than that the
					// test times out, since we're testing that something
					// DOESN'T happen but we have no way to detect when it WOULD
					// have happened if the system was behaving incorrectly.
					Expect(err).To(Equal(context.DeadlineExceeded))
					Expect(count).To(Equal(1))
				})
			})

			When("the server sends an invalid identity", func() {
				BeforeEach(func() {
					done := make(chan struct{})

					go func() {
						defer close(done)

						res := &discoverspec.WatchApplicationsResponse{
							Identity:  &discoverspec.Identity{}, // note: empty identity is invalid
							Available: true,
						}

						select {
						case <-ctx.Done():
						case responses <- res:
						}
					}()

					DeferCleanup(func() {
						<-done
					})
				})

				It("does not invoke the observer", func() {
					err := discoverer.DiscoverApplications(
						ctx,
						target,
						func(
							context.Context,
							Application,
						) {
							Fail("unexpected call")
						},
					)

					Expect(err).To(Equal(context.DeadlineExceeded))
				})

				It("logs the error", func() {
					discoverer.LogError = func(
						t Target,
						err error,
					) {
						defer cancel()

						Expect(t).To(Equal(target))
						Expect(err).To(MatchError(`invalid application identity: invalid name "", names must be non-empty, printable UTF-8 strings with no whitespace`))
					}

					err := discoverer.DiscoverApplications(ctx, target, nil)
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
						t Target,
						err error,
					) {
						defer cancel()

						Expect(t).To(Equal(target))
						Expect(err).To(MatchError(`unable to read from stream: rpc error: code = Unknown desc = <error>`))
					}

					err := discoverer.DiscoverApplications(ctx, target, nil)
					Expect(err).To(Equal(context.Canceled))
				})
			})

			When("the server can not be dialed", func() {
				BeforeEach(func() {
					// Remove the insecure transport credentials option, which
					// will cause the dialer to fail.
					target.DialOptions = nil
				})

				It("logs the error", func() {
					discoverer.LogError = func(
						t Target,
						err error,
					) {
						defer cancel()

						Expect(t).To(Equal(target))
						Expect(err).To(MatchError(`unable to dial target: grpc: no transport security set (use grpc.WithTransportCredentials(insecure.NewCredentials()) explicitly or set credentials)`))
					}

					err := discoverer.DiscoverApplications(ctx, target, nil)
					Expect(err).To(Equal(context.Canceled))
				})
			})
		})

		When("the server does not implement the discovery API", func() {
			BeforeEach(func() {
				done := make(chan struct{})
				go func() {
					defer close(done)
					gserver.Serve(listener)
				}()

				DeferCleanup(func() {
					listener.Close()
					<-done
				})
			})

			It("returns nil immediately", func() {
				err := discoverer.DiscoverApplications(ctx, target, nil)
				Expect(err).ShouldNot(HaveOccurred())
			})
		})
	})
})

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
