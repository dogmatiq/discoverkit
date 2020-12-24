package discoverkit

import (
	"context"

	"google.golang.org/grpc"
)

// A Connector establishes connections to discovered gRPC targets.
//
// It implements the DiscoverObserver and forwards to a ConnectObserver.
type Connector struct {
	// Observer is the observer to notify about gRPC connections.
	Observer ConnectObserver

	// Dial is the function used to dial gRPC targets.
	//
	// If it is nil, grpc.Dial() is used.
	Dial func(string, ...grpc.DialOption) (*grpc.ClientConn, error)

	// DialOptions is a set of default gRPC dial options that are applied
	// before each target's own dial options.
	DialOptions []grpc.DialOption

	// Ignore is a predicate function that returns true if the given target
	// should be ignored.
	//
	// Ignored targets are never dialed, and hence the observer is never
	// notified about them.
	//
	// If it is nil, no targets are ignored.
	Ignore func(context.Context, Target) (bool, error)
}

// TargetDiscovered is called when a new target is discovered.
//
// ctx is canceled if the target is undiscovered while TargetDiscovered() is
// still executing.
func (c *Connector) TargetDiscovered(ctx context.Context, t Target) error {
	if c.Ignore != nil {
		ignore, err := c.Ignore(ctx, t)
		if ignore || err != nil {
			return err
		}
	}

	dial := c.Dial
	if dial == nil {
		dial = grpc.Dial
	}

	var options []grpc.DialOption
	options = append(options, c.DialOptions...)
	options = append(options, t.DialOptions...)

	conn, err := dial(t.Name, options...)
	if err != nil {
		return err
	}
	defer conn.Close()

	return c.Observer.TargetConnected(ctx, t, conn)
}

// ConnectObserver is an interface for handling new target connections.
type ConnectObserver interface {
	// TargetConnected is called when a new connection is established.
	//
	// ctx is canceled if the target is undiscovered while TargetConnected() is
	// still executed.
	//
	// The connection is automatically closed when TargetConnected() returns.
	TargetConnected(ctx context.Context, t Target, conn grpc.ClientConnInterface) error
}
