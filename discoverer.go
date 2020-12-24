package discoverkit

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
)

// Discoverer is an interface for services that discover gRPC targets.
//
// A "target" is some endpoint that can be dialed using gRPC. It is typically a
// single gRPC server, but may be anything that can be referred to by a "name"
// as defined in https://github.com/grpc/grpc/blob/master/doc/naming.md.
type Discoverer interface {
	// Discover invokes o.TargetDiscovered() when a new target is discovered.
	//
	// Each invocation is made on its own goroutine. The context passed to
	// o.TargetDiscovered() is canceled when the target is "undiscovered", or
	// the discoverer itself is stopped due to cancelation of ctx.
	//
	// The discoverer stops and returns a DiscoverObserverError if any call to
	// o.TargetDiscovered() returns a non-nil error.
	Discover(ctx context.Context, o DiscoverObserver) error
}

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

// DiscoverObserver is an interface for handling the discovery of a target.
type DiscoverObserver interface {
	// TargetDiscovered is called when a new target is discovered.
	//
	// ctx is canceled if the target is undiscovered while TargetDiscovered() is
	// still executing.
	TargetDiscovered(ctx context.Context, t Target) error
}

// DiscoverObserverError indicates that a discoverer was stopped because a
// DiscoverObserver produced an error.
type DiscoverObserverError struct {
	Discoverer Discoverer
	Observer   DiscoverObserver
	Target     Target
	Cause      error
}

func (e DiscoverObserverError) Unwrap() error {
	return e.Cause
}

func (e DiscoverObserverError) Error() string {
	return fmt.Sprintf(
		"failure observing '%s' target: %s",
		e.Target.Name,
		e.Cause,
	)
}

// targetDiscovered calls o.TargetDiscovered().
//
// If o.TargetDiscovered() returns a non-nil error it returns a
// DiscoverObserverError.
//
// If o.TargetDiscovered() returns a context.Canceled error *and* ctx is
// canceled, it returns nil.
func targetDiscovered(
	ctx context.Context,
	d Discoverer,
	o DiscoverObserver,
	t Target,
) error {
	err := o.TargetDiscovered(ctx, t)

	if err == nil {
		return nil
	}

	if err == context.Canceled && ctx.Err() == context.Canceled {
		return nil
	}

	return DiscoverObserverError{
		Discoverer: d,
		Observer:   o,
		Target:     t,
		Cause:      err,
	}
}
