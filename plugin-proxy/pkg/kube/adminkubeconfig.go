package kube

import (
	"context"
	"fmt"
	"net/http"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// AdminKubeconfigCache yields an HTTP RoundTripper and API-server base URL for
// a target cluster.
//
// Production shape: keyed by clusterID, backed by a Gardener client with
// singleflight caching. That path is not yet wired.
//
// Local-sandbox shape: constructed from a single kubeconfig file that already
// carries admin credentials for a specific cluster (e.g. the k3d-fundament-plugin
// cluster). Every clusterID resolves to the same transport. Meant for driving
// plugin-proxy against a locally-running plugin sandbox during development.
type AdminKubeconfigCache struct {
	// sandboxTransport / sandboxHost, when set, are returned for every
	// HTTPClientFor call. Empty in production.
	sandboxTransport http.RoundTripper
	sandboxHost      string
}

// NewAdminKubeconfigCache returns an empty cache — production callers will fill
// it once the Gardener client is wired.
func NewAdminKubeconfigCache() *AdminKubeconfigCache {
	return &AdminKubeconfigCache{}
}

// NewAdminKubeconfigCacheFromFile loads a single kubeconfig from disk and pins
// every HTTPClientFor call to it. Local-dev shortcut for the plugin sandbox.
func NewAdminKubeconfigCacheFromFile(path string) (*AdminKubeconfigCache, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig %q: %w", path, err)
	}
	rt, err := rest.TransportFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("build transport from kubeconfig %q: %w", path, err)
	}
	return &AdminKubeconfigCache{sandboxTransport: rt, sandboxHost: cfg.Host}, nil
}

// HTTPClientFor returns a transport and base URL for the cluster's API server.
func (c *AdminKubeconfigCache) HTTPClientFor(_ context.Context, _ string) (http.RoundTripper, string, error) {
	if c.sandboxTransport != nil {
		return c.sandboxTransport, c.sandboxHost, nil
	}
	return nil, "", fmt.Errorf("real-mode admin kubeconfig not wired; set PLUGIN_SANDBOX_KUBECONFIG for local dev")
}
