package main

import (
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

// minKubernetesMinor is the lowest Kubernetes 1.x minor the default Envoy Gateway
// version supports. Envoy Gateway v1.8's bundled Gateway API CRDs (e.g. TLSRoute)
// carry a CEL validation rule using the isIP() function, which the API server only
// provides from Kubernetes 1.31. On older clusters the CRD is rejected as invalid
// and the install otherwise fails with a cryptic
// "missing CRDs: [tlsroutes.gateway.networking.k8s.io]".
const minKubernetesMinor = 31

// serverVersioner is the slice of client-go's discovery client the preflight
// needs, as an interface so the check is unit-testable without a live cluster.
type serverVersioner interface {
	ServerVersion() (*version.Info, error)
}

// newDiscoveryClient builds a discovery client from the rest config.
func newDiscoveryClient(cfg *rest.Config) (*discovery.DiscoveryClient, error) {
	return discovery.NewDiscoveryClientForConfig(cfg)
}

// checkKubernetesVersion fails fast, with an actionable message, when the cluster
// is older than Envoy Gateway supports — rather than letting the CRD apply fail
// deep inside the Helm install with an opaque error.
func checkKubernetesVersion(sv serverVersioner) error {
	info, err := sv.ServerVersion()
	if err != nil {
		return fmt.Errorf("get kubernetes server version: %w", err)
	}
	ok, err := kubernetesMinorAtLeast(info.GitVersion, minKubernetesMinor)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf(
			"the Envoy Gateway plugin requires Kubernetes >= 1.%d, but the cluster is %s — upgrade the cluster before installing",
			minKubernetesMinor, info.GitVersion,
		)
	}
	return nil
}

// kubernetesMinorAtLeast reports whether the server's git version is at least
// 1.<minMinor>. GitVersion looks like "v1.30.6+k3s1" / "v1.31.5-eks-..."; only
// the major and minor components are parsed, tolerating a leading "v" and any
// build-metadata suffix on the minor.
func kubernetesMinorAtLeast(gitVersion string, minMinor int) (bool, error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(gitVersion), "v")
	parts := strings.SplitN(trimmed, ".", 3)
	if len(parts) < 2 {
		return false, fmt.Errorf("unexpected kubernetes version %q", gitVersion)
	}
	major, err := strconv.Atoi(leadingDigits(parts[0]))
	if err != nil {
		return false, fmt.Errorf("parse kubernetes major version from %q: %w", gitVersion, err)
	}
	if major > 1 {
		return true, nil
	}
	minor, err := strconv.Atoi(leadingDigits(parts[1]))
	if err != nil {
		return false, fmt.Errorf("parse kubernetes minor version from %q: %w", gitVersion, err)
	}
	return major == 1 && minor >= minMinor, nil
}

// leadingDigits returns the leading run of ASCII digits in s (e.g. "31+" -> "31").
func leadingDigits(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
}
