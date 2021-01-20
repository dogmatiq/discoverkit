package discoverkit

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/dogmatiq/linger"
)

const (
	// DefaultDNSQueryInterval is the default interval at which DNS queries are
	// performed.
	DefaultDNSQueryInterval = 10 * time.Second
)

// DNSTargetDiscoverer is a TargetDiscoverer that performs a DNS query to
// discover targets.
//
// It queries a single host and treats each A, AAAA or CNAME record in the
// result as a distinct target. This is not a DNS-SD implementation.
type DNSTargetDiscoverer struct {
	// QueryHost is the hostname that is queried.
	QueryHost string

	// NewTargets returns the targets that are discovered based on the addition
	// of a new network address to the DNS query result.
	//
	// addr is the address discovered by the DNS query. It may be a hostname or
	// an IP address.
	//
	// If NewTargets is nil the discoverer constructs a single Target for each
	// discovered address. The target name is built using the discovered address
	// as the host and the DefaultGRPCPort constant for the port.
	NewTargets func(ctx context.Context, addr string) (targets []Target, err error)

	// LookupHost is the function used to query the host.
	//
	// If it is nil, net.DefaultResolver.LookupHost() is used.
	LookupHost func(ctx context.Context, host string) (addresses []string, err error)

	// QueryInterval is the interval at which DNS queries are performed.
	//
	// If it is non-positive, the DefaultDNSQueryInterval constant is used.
	QueryInterval time.Duration
}

// DiscoverTargets invokes an observer for each gRPC target that is discovered.
//
// It runs until ctx is canceled or an error occurs.
//
// The context passed to the observer is canceled when the target becomes
// unavailable or the discover is stopped.
//
// The discoverer MAY block on calls to the observer. It is the observer's
// responsibility to start new goroutines to handle background tasks, as
// appropriate.
func (d *DNSTargetDiscoverer) DiscoverTargets(ctx context.Context, obs TargetObserver) error {
	addresses := map[string]context.CancelFunc{}

	defer func() {
		for _, cancel := range addresses {
			cancel()
		}
	}()

	for {
		// Perform the DNS query.
		results, err := d.query(ctx)
		if err != nil {
			return err
		}

		// Invoke the observer / cancel contexts to sync the observer state with
		// the new results.
		if err := d.sync(ctx, addresses, results, obs); err != nil {
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
func (d *DNSTargetDiscoverer) sync(
	ctx context.Context,
	addresses map[string]context.CancelFunc,
	results map[string]struct{},
	obs TargetObserver,
) error {
	// First we check through the known addresses to work out which ones are
	// still in the latest query results.
	for addr, cancel := range addresses {
		if _, ok := results[addr]; ok {
			// This address is still avaliable. Remove it from the query results
			// so we're left only with addresses that we have not seen before.
			delete(results, addr)
		} else {
			// This address is no longer in the results. Cancel the associated
			// context to stop the observer goroutines.
			delete(addresses, addr)
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

		// Create a new context specifically for this address. It will be
		// canceled if the address dissappears from the query results.
		addrCtx, cancel := context.WithCancel(ctx)
		addresses[addr] = cancel

		// Invoke the observer for each target.
		for _, t := range targets {
			obs(addrCtx, t)
		}
	}

	return nil
}

// query performs a DNS query to A, AAAA and CNAME records associated with
// d.QueryHost.
//
// It returns the resulting addresses as a set with names transformed to
// lowercase. Individual addresses may be hostnames or IP addresses.
func (d *DNSTargetDiscoverer) query(ctx context.Context) (map[string]struct{}, error) {
	lookupHost := d.LookupHost
	if lookupHost == nil {
		lookupHost = net.DefaultResolver.LookupHost
	}

	addrs, err := lookupHost(ctx, d.QueryHost)
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
func (d *DNSTargetDiscoverer) newTargets(ctx context.Context, addr string) ([]Target, error) {
	if d.NewTargets != nil {
		return d.NewTargets(ctx, addr)
	}

	return []Target{
		{
			Name: net.JoinHostPort(addr, DefaultGRPCPort),
		},
	}, nil
}
