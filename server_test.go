package discoverkit_test

import (
	"context"
	"net"
	"time"

	"github.com/dogmatiq/configkit"
	. "github.com/dogmatiq/discoverkit"
	"github.com/dogmatiq/interopspec/discoverspec"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var _ = Describe("type Server", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc

		app1, app2, app3 configkit.Identity
		server           *Server

		listener net.Listener
		gserver  *grpc.Server
		conn     *grpc.ClientConn
		cli      discoverspec.DiscoverAPIClient
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)

		app1 = configkit.MustNewIdentity("<app-1-name>", "<app-1-key>")
		app2 = configkit.MustNewIdentity("<app-2-name>", "<app-2-key>")
		app3 = configkit.MustNewIdentity("<app-3-name>", "<app-3-key>")

		server = &Server{}

		var err error
		listener, err = net.Listen("tcp", ":")
		Expect(err).ShouldNot(HaveOccurred())

		gserver = grpc.NewServer()
		discoverspec.RegisterDiscoverAPIServer(gserver, server)

		go gserver.Serve(listener)

		conn, err = grpc.Dial(
			listener.Addr().String(),
			grpc.WithTransportCredentials(
				insecure.NewCredentials(),
			),
		)
		Expect(err).ShouldNot(HaveOccurred())

		cli = discoverspec.NewDiscoverAPIClient(conn)
	})

	AfterEach(func() {
		cancel()

		if conn != nil {
			conn.Close()
		}

		if gserver != nil {
			gserver.Stop()
		}

		if listener != nil {
			listener.Close()
		}
	})

	Describe("func Available()", func() {
		It("notifies new watchers that the application is available", func() {
			server.Available(app1)

			stream, err := cli.WatchApplications(ctx, &discoverspec.WatchApplicationsRequest{})
			Expect(err).ShouldNot(HaveOccurred())

			m, err := stream.Recv()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(m.Identity.Name).To(Equal("<app-1-name>"))
			Expect(m.Identity.Key).To(Equal("<app-1-key>"))
			Expect(m.Available).To(BeTrue())
		})

		It("notifies existing watchers that the application is available", func() {
			stream, err := cli.WatchApplications(ctx, &discoverspec.WatchApplicationsRequest{})
			Expect(err).ShouldNot(HaveOccurred())

			go func() {
				time.Sleep(20 * time.Millisecond)
				server.Available(app1)
			}()

			m, err := stream.Recv()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(m.Identity.Name).To(Equal("<app-1-name>"))
			Expect(m.Identity.Key).To(Equal("<app-1-key>"))
			Expect(m.Available).To(BeTrue())
		})

		It("does not notify watchers if the application is already available", func() {
			server.Available(app1)

			stream, err := cli.WatchApplications(ctx, &discoverspec.WatchApplicationsRequest{})
			Expect(err).ShouldNot(HaveOccurred())

			_, err = stream.Recv() // read "available" notification
			Expect(err).ShouldNot(HaveOccurred())

			server.Available(app1)

			// Annoyingly, stream.Recv() can represent a deadline error in at
			// least 3 different ways, so we just perform a substring match.
			_, err = stream.Recv()
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context deadline exceeded"))
		})

		It("does not block if there are slow consumers", func() {
			// Open the stream but never read from it.
			_, err := cli.WatchApplications(ctx, &discoverspec.WatchApplicationsRequest{})
			Expect(err).ShouldNot(HaveOccurred())

			server.Available(app1)
		})
	})

	Describe("func Unavailable()", func() {
		It("does not notify new watchers that the application is unavailable", func() {
			server.Available(app1)
			server.Unavailable(app1)

			stream, err := cli.WatchApplications(ctx, &discoverspec.WatchApplicationsRequest{})
			Expect(err).ShouldNot(HaveOccurred())

			// Annoyingly, stream.Recv() can represent a deadline error in at
			// least 3 different ways, so we just perform a substring match.
			_, err = stream.Recv()
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context deadline exceeded"))
		})

		It("notifies existing watchers that the application is unavailable", func() {
			server.Available(app1)

			stream, err := cli.WatchApplications(ctx, &discoverspec.WatchApplicationsRequest{})
			Expect(err).ShouldNot(HaveOccurred())

			_, err = stream.Recv() // read "available" notification
			Expect(err).ShouldNot(HaveOccurred())

			server.Unavailable(app1)

			m, err := stream.Recv()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(m.Identity.Name).To(Equal("<app-1-name>"))
			Expect(m.Identity.Key).To(Equal("<app-1-key>"))
			Expect(m.Available).To(BeFalse())
		})

		It("does not block if there are slow consumers", func() {
			server.Available(app1)

			// Open the stream but never read from it.
			_, err := cli.WatchApplications(ctx, &discoverspec.WatchApplicationsRequest{})
			Expect(err).ShouldNot(HaveOccurred())

			server.Unavailable(app1)
		})
	})

	Describe("func WatchApplications()", func() {
		It("sends the current state when a call is first made", func() {
			server.Available(app1)
			server.Available(app2)
			server.Unavailable(app1)
			server.Available(app3)

			stream, err := cli.WatchApplications(ctx, &discoverspec.WatchApplicationsRequest{})
			Expect(err).ShouldNot(HaveOccurred())

			available := map[string]struct{}{}

			for {
				m, err := stream.Recv() // read "available" notification
				if err != nil {
					break
				}

				if m.Available {
					available[m.Identity.Key] = struct{}{}
				} else {
					delete(available, m.Identity.Key)
				}
			}

			Expect(available).To(Equal(
				map[string]struct{}{
					app2.Key: {},
					app3.Key: {},
				},
			))
		})

		It("sends diffs as updates occur", func() {
			stream, err := cli.WatchApplications(ctx, &discoverspec.WatchApplicationsRequest{})
			Expect(err).ShouldNot(HaveOccurred())

			available := map[string]struct{}{}

			go func() {
				server.Available(app1)
				time.Sleep(5 * time.Millisecond)
				server.Available(app2)
				time.Sleep(5 * time.Millisecond)
				server.Unavailable(app1)
				time.Sleep(5 * time.Millisecond)
				server.Available(app3)
			}()

			for {
				m, err := stream.Recv() // read "available" notification
				if err != nil {
					break
				}

				if m.Available {
					available[m.Identity.Key] = struct{}{}
				} else {
					delete(available, m.Identity.Key)
				}
			}

			Expect(available).To(Equal(
				map[string]struct{}{
					app2.Key: {},
					app3.Key: {},
				},
			))
		})
	})

})
