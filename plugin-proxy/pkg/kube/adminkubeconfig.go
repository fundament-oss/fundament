// Package kube adapts the shared Gardener admin-kubeconfig cache to the
// shapes plugin-proxy's asset fetcher and installation backend consume.
package kube

import (
	"context"
	"fmt"
	"net/http"

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
// a target cluster, backed by common/gardener's cached short-lived admin
// kubeconfigs.
type AdminKubeconfigCache struct {
	cache *gardener.AdminKubeconfigCache
}

var _ KubeconfigSource = (*AdminKubeconfigCache)(nil)

// NewAdminKubeconfigCache wraps the shared cache.
func NewAdminKubeconfigCache(cache *gardener.AdminKubeconfigCache) *AdminKubeconfigCache {
	return &AdminKubeconfigCache{cache: cache}
}

// HTTPClientFor returns a transport and base URL for the cluster's API server.
func (c *AdminKubeconfigCache) HTTPClientFor(ctx context.Context, clusterID string) (http.RoundTripper, string, error) {
	access, err := c.cache.AccessFor(ctx, clusterID)
	if err != nil {
		return nil, "", fmt.Errorf("shoot access: %w", err)
	}
	return access.Transport, access.Host.String(), nil
}
