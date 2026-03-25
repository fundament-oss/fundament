package kube

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/gardener"
)

const kubeconfigCacheTTL = 45 * time.Minute

// MultiClusterProxy routes Kubernetes API requests to the correct cluster
// based on the Fun-Cluster header. It lazily creates one httputil.ReverseProxy
// per cluster by fetching a short-lived admin kubeconfig from Gardener,
// caching it for kubeconfigCacheTTL before re-fetching.
type MultiClusterProxy struct {
	gardener *gardener.Client
	logger   *slog.Logger
	proxies  sync.Map           // string(clusterID) → *cachedProxy
	group    singleflight.Group // deduplicates concurrent proxy construction for the same cluster
}

type cachedProxy struct {
	proxy     *httputil.ReverseProxy
	expiresAt time.Time
}

// NewMultiClusterProxy returns a MultiClusterProxy that fetches kubeconfigs
// from Gardener on demand using the provided gardener.Client.
func NewMultiClusterProxy(gc *gardener.Client, logger *slog.Logger) *MultiClusterProxy {
	return &MultiClusterProxy{
		gardener: gc,
		logger:   logger,
	}
}

func (m *MultiClusterProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	clusterID := r.Header.Get("Fun-Cluster")
	if clusterID == "" {
		http.Error(w, "missing cluster header", http.StatusBadRequest)
		return
	}

	proxy, err := m.proxyFor(r.Context(), clusterID)
	if err != nil {
		m.logger.ErrorContext(r.Context(), "failed to build proxy for cluster", "cluster", clusterID, "error", err)
		http.Error(w, "failed to contact kubernetes API", http.StatusBadGateway)
		return
	}

	proxy.ServeHTTP(w, r)
}

func (m *MultiClusterProxy) proxyFor(ctx context.Context, clusterID string) (*httputil.ReverseProxy, error) {
	if v, ok := m.proxies.Load(clusterID); ok {
		cp := v.(*cachedProxy)
		if time.Now().Before(cp.expiresAt) {
			return cp.proxy, nil
		}
	}

	v, err, _ := m.group.Do(clusterID, func() (any, error) {
		return m.buildProxy(ctx, clusterID)
	})
	if err != nil {
		return nil, err
	}
	return v.(*httputil.ReverseProxy), nil
}

func (m *MultiClusterProxy) buildProxy(ctx context.Context, clusterID string) (*httputil.ReverseProxy, error) {
	kubeconfigData, err := m.gardener.GetAdminKubeconfig(ctx, clusterID, 0)
	if err != nil {
		return nil, fmt.Errorf("get admin kubeconfig for cluster %s: %w", clusterID, err)
	}

	c, err := NewFromBytes(kubeconfigData)
	if err != nil {
		return nil, fmt.Errorf("build client for cluster %s: %w", clusterID, err)
	}

	proxy := buildReverseProxy(c.Host(), c.Transport(), m.logger)
	m.proxies.Store(clusterID, &cachedProxy{
		proxy:     proxy,
		expiresAt: time.Now().Add(kubeconfigCacheTTL),
	})
	return proxy, nil
}

func buildReverseProxy(target *url.URL, transport http.RoundTripper, logger *slog.Logger) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Transport: transport,
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
			// Strip client auth headers so the kubeconfig transport supplies its own.
			req.Header.Del("Authorization")
			req.Header.Del("Cookie")
			req.Header.Del("Fun-Organization")
			req.Header.Del("Fun-Cluster")
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			logger.ErrorContext(req.Context(), "kubernetes proxy error", "error", err)
			http.Error(w, "failed to contact kubernetes API", http.StatusBadGateway)
		},
	}
}
