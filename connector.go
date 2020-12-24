package discoverkit

import (
	"context"

	"google.golang.org/grpc"
)

// Connector is a DiscoverObserver implementation that dials each discovered
// target and notifies a ConnectObserver when connections become available or
// unavailable.
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
	Ignore func(Target) bool
}

// TargetDiscovered is called when a new target is discovered.
//
// ctx is canceled if the target is undiscovered while TargetDiscovered() is
// still executing.
func (c *Connector) TargetDiscovered(ctx context.Context, t Target) error {
	if c.Ignore != nil && c.Ignore(t) {
		return nil
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
