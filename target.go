package discoverkit

import (
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

	// Options is a set of grpc.DialOptions used when dialing this target.
	//
	// The options MUST NOT include grpc.WithBlock() or
	// grpc.WithReturnConnectionError().
	Options []grpc.DialOption
}
