package kube

import (
	"context"
	"fmt"
	"net/http"
)

// AdminKubeconfigCache yields an HTTP RoundTripper and API-server base URL for
// a target cluster. Mirrors kube-api-proxy/pkg/kube; a shared module is a
// follow-up (see the spec's "Open implementation choices").
type AdminKubeconfigCache struct {
	// fields & singleflight caching, copied from kube-api-proxy
}

func NewAdminKubeconfigCache( /* gardener client, logger */ ) *AdminKubeconfigCache {
	return &AdminKubeconfigCache{}
}

// HTTPClientFor returns a transport and base URL for the cluster's API server.
func (c *AdminKubeconfigCache) HTTPClientFor(_ context.Context, _ string) (http.RoundTripper, string, error) {
	return nil, "", fmt.Errorf("real-mode admin kubeconfig not wired; mock mode only in this plan")
}
