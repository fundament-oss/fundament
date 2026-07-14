// Package kube adapts the shared Gardener admin-kubeconfig cache to the
// shapes plugin-proxy's asset fetcher and installation backend consume.
package kube

import (
	"context"
	"fmt"
	"net/http"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/fundament-oss/fundament/common/gardener"
)

// KubeconfigSource yields an HTTP transport and API-server base URL for a
// target cluster. Implemented by AdminKubeconfigCache; declared as an
// interface here (the single definition consumed by the asset fetcher and the
// installation backend) so those consumers can be faked in tests.
type KubeconfigSource interface {
	HTTPClientFor(ctx context.Context, clusterID string) (http.RoundTripper, string, error)
}

// AdminKubeconfigCache yields an HTTP RoundTripper and API-server base URL for
// a target cluster.
//
// Production shape: backed by common/gardener's cached short-lived admin
// kubeconfigs, keyed by clusterID.
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

	// gardenerCache, when set, resolves clusterID → shoot admin access via
	// the shared Gardener cache. Nil in the sandbox shape.
	gardenerCache *gardener.AdminKubeconfigCache
}

var _ KubeconfigSource = (*AdminKubeconfigCache)(nil)

// NewAdminKubeconfigCache returns an empty cache — every HTTPClientFor call
// fails until a Gardener cache or sandbox kubeconfig is wired.
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

// NewAdminKubeconfigCacheFromGardener wraps the shared Gardener cache —
// the production (real-mode) shape.
func NewAdminKubeconfigCacheFromGardener(cache *gardener.AdminKubeconfigCache) *AdminKubeconfigCache {
	return &AdminKubeconfigCache{gardenerCache: cache}
}

// HTTPClientFor returns a transport and base URL for the cluster's API server.
func (c *AdminKubeconfigCache) HTTPClientFor(ctx context.Context, clusterID string) (http.RoundTripper, string, error) {
	if c.sandboxTransport != nil {
		return c.sandboxTransport, c.sandboxHost, nil
	}
	if c.gardenerCache != nil {
		access, err := c.gardenerCache.AccessFor(ctx, clusterID)
		if err != nil {
			return nil, "", fmt.Errorf("shoot access: %w", err)
		}
		return access.Transport, access.Host.String(), nil
	}
	return nil, "", fmt.Errorf("real-mode admin kubeconfig not wired; set PLUGIN_SANDBOX_KUBECONFIG for local dev")
}
