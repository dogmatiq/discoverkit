package discoverkit

import (
	"context"
	"net"

	"github.com/dogmatiq/configkit"
	"google.golang.org/grpc"
)

// Target represents some dialable gRPC target, typically a single gRPC server.
type Target struct {
	// Name is the target name used to dial the target.
	//
	// Typically this is the hostname and port of a single gRPC server but it
	// may use any of the naming schemes defined in
	// https://github.com/grpc/grpc/blob/master/doc/naming.md.
	Name string

	// DialOptions is a set of grpc.DialOptions used when dialing this target.
	DialOptions []grpc.DialOption
}

// TargetObserver is a function that handles the discovery of a gRPC target.
//
// ctx is canceled when the target becomes unavailable or the discoverer is
// stopped. ctx is NOT canceled when the observer function returns and as such
// may be used by goroutines started by the observer.
//
// The discoverer MAY block on calls to the observer. It is the observer's
// responsibility to start new goroutines to handle background tasks, as
// appropriate.
type TargetObserver func(ctx context.Context, t Target)

// TargetDiscoverer is an interface for services that advertise and discover
// gRPC targets.
//
// A "target" is some endpoint that can be dialed using gRPC. It is typically a
// single gRPC server, but may be anything that can be referred to by a "name"
// as defined in https://github.com/grpc/grpc/blob/master/doc/naming.md.
type TargetDiscoverer interface {
	// DiscoverTargets invokes an observer for each gRPC target that is
	// discovered.
	//
	// It runs until ctx is canceled or an error occurs.
	//
	// The context passed to the observer is canceled when the target becomes
	// unavailable or the discover is stopped.
	//
	// The discoverer MAY block on calls to the observer. It is the observer's
	// responsibility to start new goroutines to handle background tasks, as
	// appropriate.
	DiscoverTargets(ctx context.Context, obs TargetObserver) error

	// AdvertiseTarget advertises a target so that it may be discovered by a
	// TargetDiscoverer.
	//
	// addr is the address on which the gRPC server accepts connections.
	//
	// It runs until ctx is canceled or an error occurs. If this discovery
	// method does not require advertisement, it returns nil immediately.
	AdvertiseTarget(
		ctx context.Context,
		addr net.Addr,
		applications []configkit.Identity,
	) error
}
