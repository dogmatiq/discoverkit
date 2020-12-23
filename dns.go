package discoverkit

import (
	"context"
	"net"
	"time"

	"github.com/dogmatiq/linger"
)

// DNSResolver is an interface for the subset of net.Resolver used by
// DNSDiscoverer.
type DNSResolver interface {
	LookupHost(ctx context.Context, host string) ([]string, error)
}

const (
	// DefaultDNSQueryInterval is the default interval at which DNS queries are
	// performed.
	DefaultDNSQueryInterval = 10 * time.Second
)

// DNSDiscoverer is a Discoverer that performs a DNS query to discover targets.
//
// It queries a single host and treats each A, AAAA or CNAME record in the
// result as a distinct target. This is not a DNS-SD implementation.
type DNSDiscoverer struct {
	// QueryHost is the hostname that is queried.
	QueryHost string

	// NewTargets returns the targets that are discovered based on the addition
	// of a new network address to the DNS query result.
	//
	// addr is the address discovered by the DNS query. It may be a hostname or
	// an IP address.
	//
	// If NewTargets is nil the discoverer constructs a single Target for each
	// discovered address. The target name is the discovered address and no
	// explicit port is specified.
	NewTargets func(ctx context.Context, addr string) (targets []Target, err error)

	// Resolver is the DNS resolver used to make queries.
	//
	// If it is nil, net.DefaultResolver is used.
	Resolver DNSResolver

	// QueryInterval is the interval at which DNS queries are performed.
	//
	// If it is non-positive, the DefaultDNSQueryInterval constant is used.
	QueryInterval time.Duration

	// targets is the set of targets currently known to the discoverer.
	targets map[string][]DiscoveredTarget
}

// Discover notifies o of targets that are discovered or undiscovered until ctx
// is canceled or an error occurs.
func (d *DNSDiscoverer) Discover(ctx context.Context, o TargetObserver) error {
	defer func() {
		for _, targets := range d.targets {
			for _, t := range targets {
				o.TargetUndiscovered(t)
			}
		}
	}()

	for {
		addrs, err := d.query(ctx)
		if err != nil {
			return err
		}

		if err := d.update(ctx, addrs, o); err != nil {
			return err
		}

		if err := linger.Sleep(
			ctx,
			d.QueryInterval,
			DefaultDNSQueryInterval,
		); err != nil {
			return err
		}
	}
}

// update sends notifications to o about the targets that have become
// available/unavailable based on new query results.
func (d *DNSDiscoverer) update(
	ctx context.Context,
	addrs []string,
	o TargetObserver,
) error {
	prev := d.targets
	d.targets = make(map[string][]DiscoveredTarget, len(addrs))

	// Add an entry to d.targets for each of the addresses in the query result.
	for _, addr := range addrs {
		if targets, ok := prev[addr]; ok {
			// We have already discovered this address. It may or may not have
			// any targets. Either way we retain them unchanged.
			d.targets[addr] = targets
			continue
		}

		// Otherwise, we're seeing this address for the first time, or it's
		// reappeared after being removed from the DNS results. We construct the
		// targets that are at this address.
		targets, err := d.newTargets(ctx, addr)
		if err != nil {
			return err
		}

		if len(targets) == 0 {
			// Explicitly store a nil slice against this address if there are no
			// targets. This ensures that we still treat the address as "seen".
			d.targets[addr] = nil
			continue
		}

		// Make a DiscoveredTarget for each Target and notify the observer.
		discovered := make([]DiscoveredTarget, len(targets))
		for i, t := range targets {
			dt := DiscoveredTarget{
				Target:     t,
				ID:         DiscoveredTargetID(),
				Discoverer: d,
			}

			discovered[i] = dt
			o.TargetDiscovered(dt)
		}

		// Store the discovered targets against the address.
		d.targets[addr] = discovered
	}

	// Notify the observer of any targets that have gone away because the
	// address they were based on is no longer in the query results.
	for a, targets := range prev {
		if _, ok := d.targets[a]; !ok {
			for _, dt := range targets {
				o.TargetUndiscovered(dt)
			}
		}
	}

	return nil
}

// query performs a DNS query to find API targets.
func (d *DNSDiscoverer) query(ctx context.Context) ([]string, error) {
	r := d.Resolver
	if r == nil {
		r = net.DefaultResolver
	}

	addrs, err := r.LookupHost(ctx, d.QueryHost)
	if err != nil {
		if x, ok := err.(*net.DNSError); ok {
			if x.IsTemporary || x.IsNotFound {
				return nil, nil
			}
		}

		return nil, err
	}

	return addrs, nil
}

// newTarget returns the targets at the given address.
func (d *DNSDiscoverer) newTargets(ctx context.Context, addr string) ([]Target, error) {
	if d.NewTargets != nil {
		return d.NewTargets(ctx, addr)
	}

	return []Target{
		{Name: addr},
	}, nil
}
