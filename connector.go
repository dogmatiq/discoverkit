package discoverkit

import (
	"sync"

	"google.golang.org/grpc"
)

// Connector is a TargetObserver implementation that dials each discovered
// target and notifies a ConnectionObserver when connections become available or
// unavailable.
type Connector struct {
	// Observer is the observer to notify about gRPC connections.
	Observer ConnectionObserver

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
	Ignore func(*Target) bool

	// OnDialError is a function that is called when a call to Dial() fails.
	//
	// Typically dialing only fails if there's some problem with the dial
	// options. By default, underlying connection is established lazily *after*
	// the dialer returns.
	//
	// If it is nil, dialing errors are silently ignored.
	OnDialError func(*Target, error)

	m           sync.Mutex
	connections map[*Target]*grpc.ClientConn
}

// TargetDiscovered is called when a discoverer becomes aware of a target.
//
// It attempts to dial the target and if successful it notifies the observer of
// the new connection.
func (c *Connector) TargetDiscovered(t *Target) {
	if c.Ignore != nil && c.Ignore(t) {
		return
	}

	dial := c.Dial
	if dial == nil {
		dial = grpc.Dial
	}

	var options []grpc.DialOption
	options = append(options, c.DialOptions...)
	options = append(options, t.DialOptions...)

	c.m.Lock()
	defer c.m.Unlock()

	if c.connections == nil {
		c.connections = map[*Target]*grpc.ClientConn{}
	} else if _, ok := c.connections[t]; ok {
		// This target has already been discovered. This should never occur but
		// is added to account for misbehaving Discoverer implementations.
		return
	}

	conn, err := dial(t.Name, options...)
	if err != nil {
		if c.OnDialError != nil {
			c.OnDialError(t, err)
		}

		return
	}

	c.connections[t] = conn
	c.Observer.ConnectionAvailable(t, conn)
}

// TargetUndiscovered is called when a previously discovered target is no longer
// considered to exist.
//
// If a connection has been dialed for this target the observer is first
// notified that the connection is no longer available and then the connection
// is closed.
func (c *Connector) TargetUndiscovered(t *Target) {
	c.m.Lock()
	defer c.m.Unlock()

	if conn, ok := c.connections[t]; ok {
		delete(c.connections, t)
		c.Observer.ConnectionUnavailable(t, conn)
		conn.Close()
	}
}

// ConnectionObserver is notified when a connection to a target becomes
// available or unavailable.
type ConnectionObserver interface {
	// ConnectionAvailable is called when a connection to a target becomes
	// available.
	ConnectionAvailable(*Target, grpc.ClientConnInterface)

	// ConnectionUnavailable is called when a connection to a target becomes
	// unavailable.
	ConnectionUnavailable(*Target, grpc.ClientConnInterface)
}
