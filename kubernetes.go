package discoverkit

import (
	"context"
	"net"
	"os"
	"strings"

	"google.golang.org/grpc"
)

// DefaultKubernetesPortName is the default port name used to discover
// Kubernetes services that are valid gRPC targets.
const DefaultKubernetesPortName = "dogma"

// KubernetesEnvironmentTargetDiscoverer discovers gRPC targets by inspecting
// the environment for Kubernetes-style environment variables.
type KubernetesEnvironmentTargetDiscoverer struct {
	// PortName is the name (not the number) of the port used to identify a
	// Dogma target.
	//
	// If it is empty, DefaultKubernetesPortName is used.
	PortName string

	// DialOptions returns the dial options used to dial the given address.
	DialOptions func(addr string) []grpc.DialOption
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
func (d *KubernetesEnvironmentTargetDiscoverer) DiscoverTargets(
	ctx context.Context,
	obs TargetObserver,
) error {
	portName := d.PortName
	if portName == "" {
		portName = DefaultKubernetesPortName
	}

	separator := "_SERVICE_PORT_" +
		kubernetesNameToEnv(portName) +
		"="

	for _, line := range os.Environ() {
		i := strings.Index(line, separator)
		if i < 0 {
			continue
		}

		port := line[i+len(separator):]
		if port == "" {
			continue
		}

		host := os.Getenv(line[:i] + "_SERVICE_HOST")
		if host == "" {
			continue
		}

		t := Target{
			Name: net.JoinHostPort(host, port),
		}

		if d.DialOptions != nil {
			t.DialOptions = d.DialOptions(t.Name)
		}

		obs(ctx, t)
	}

	<-ctx.Done()

	return ctx.Err()
}

// kubernetesNameToEnv converts a kubernetes resource name to an environment
// variable name, as per Kubernetes own behavior.
func kubernetesNameToEnv(s string) string {
	return strings.ToUpper(
		strings.ReplaceAll(s, "-", "_"),
	)
}
