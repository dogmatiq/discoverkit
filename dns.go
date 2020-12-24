package discoverkit

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/dogmatiq/linger"
	"golang.org/x/sync/errgroup"
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

	parent   context.Context
	group    *errgroup.Group
	known    map[string]context.CancelFunc
	observer TargetObserver
}

// Discover invokes o.TargetDiscovered() when a new target is discovered.
//
// Each invocation is made on its own goroutine. The context passed to
// o.TargetDiscovered() is canceled when the target becomes unavailable, or the
// discoverer itself is stopped due to cancelation of ctx.
//
// The discoverer stops and returns a TargetObserverError if any call to
// o.TargetDiscovered() returns a non-nil error.
func (d *DNSDiscoverer) Discover(ctx context.Context, o TargetObserver) error {
	d.group, d.parent = errgroup.WithContext(ctx)
	d.known = map[string]context.CancelFunc{}
	d.observer = o

	d.group.Go(func() error {
		// Perform the actual discovery within the same group as the observer
		// goroutines. This ensures both that Wait() always has something to
		// wait for, so that it doesn't just return immediately, and that the
		// whole group is shutdown if the discovery process itself fails.
		return d.discover(d.parent)
	})

	return d.group.Wait()
}

// discover periodically queries the DNS server and starts/stops observer
// goroutines as necessary.
func (d *DNSDiscoverer) discover(ctx context.Context) error {
	for {
		// Perform the DNS query.
		results, err := d.query(ctx)
		if err != nil {
			return err
		}

		// Start and stop observer goroutines to match the new results.
		if err := d.sync(ctx, results); err != nil {
			return err
		}

		// Wait until it's time to perform the next DNS query.
		if err := linger.Sleep(
			ctx,
			d.QueryInterval,
			DefaultDNSQueryInterval,
		); err != nil {
			return err
		}
	}
}

// sync synchronizes the state of running observers based on a new set of DNS
// query results.
func (d *DNSDiscoverer) sync(
	ctx context.Context,
	results map[string]struct{},
) error {
	// First we check through the known addresses to work out which ones are
	// still in the latest query results.
	for addr, cancel := range d.known {
		if _, ok := results[addr]; ok {
			// This address is still avaliable. Remove it from the query results
			// so we're left only with addresses that we have not seen before.
			delete(results, addr)
		} else {
			// This address is no longer in the results. Cancel the associated
			// context to stop the observer goroutines.
			delete(d.known, addr)
			cancel()
		}
	}

	// Then we can look at the query results, which at this point contains only
	// those addresses we didn't already know about.
	for addr := range results {
		targets, err := d.newTargets(ctx, addr)
		if err != nil {
			return err
		}

		// Create a new context specifically for this address. Note that it is
		// derived directly from d.parent, not ctx (which represents only the
		// lifetime of this call to the update() method). It will be canceled if
		// the address dissappears from the query results.
		addrCtx, cancel := context.WithCancel(d.parent)
		d.known[addr] = cancel

		// Start a new goroutine for each target.
		for _, t := range targets {
			t := t // capture loop variable
			d.group.Go(func() error {
				return targetDiscovered(addrCtx, d, d.observer, t)
			})
		}
	}

	return nil
}

// query performs a DNS query to A, AAAA and CNAME records associated with
// d.QueryHost.
//
// It returns the resulting addresses as a set with names transformed to
// lowercase. Individual addresses may be hostnames or IP addresses.
func (d *DNSDiscoverer) query(ctx context.Context) (map[string]struct{}, error) {
	r := d.Resolver
	if r == nil {
		r = net.DefaultResolver
	}

	addrs, err := r.LookupHost(ctx, d.QueryHost)
	if err != nil {
		if x, ok := err.(*net.DNSError); ok {
			// Temporary network problems, or the fact that host doesn't exist
			// *right now* are not errors that should stop the discoverer.
			if x.IsTemporary || x.IsNotFound {
				return nil, nil
			}
		}

		return nil, err
	}

	// Convert the slice of addresses to a set of lowercase strings.
	results := make(map[string]struct{}, len(addrs))
	for _, addr := range addrs {
		results[strings.ToLower(addr)] = struct{}{}
	}

	return results, err
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
