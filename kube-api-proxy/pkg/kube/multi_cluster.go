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
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/token"
)

// adminKubeconfigRefreshRatio is the fraction of TTL at which to proactively
// refresh admin kubeconfigs (70% of TTL).
const adminKubeconfigRefreshRatio = 0.7

// MultiClusterProxy routes Kubernetes API requests to the correct cluster
// based on the cluster ID from request context. It lazily creates one httputil.ReverseProxy
// per cluster by fetching a short-lived admin kubeconfig from Gardener,
// caching it until 70% of its TTL before re-fetching.
type MultiClusterProxy struct {
	gardener   *gardener.Client
	tokenCache *token.Cache
	logger     *slog.Logger
	proxies    sync.Map           // string(clusterID) → *cachedProxy
	group      singleflight.Group // deduplicates concurrent proxy construction for the same cluster
}

type cachedProxy struct {
	proxy     *httputil.ReverseProxy
	expiresAt time.Time
}

// NewMultiClusterProxy returns a MultiClusterProxy that fetches kubeconfigs
// from Gardener on demand using the provided gardener.Client.
func NewMultiClusterProxy(gc *gardener.Client, tc *token.Cache, logger *slog.Logger) *MultiClusterProxy {
	return &MultiClusterProxy{
		gardener:   gc,
		tokenCache: tc,
		logger:     logger,
	}
}

// Context keys shared between the proxy handler and the kube package.
// Defined here because kube cannot import proxy (would be circular).

// ClusterIDContextKey is used to pass the cluster ID from the handler to the proxy via request context.
type ClusterIDContextKey struct{}

// SATokenContextKey is used to pass the per-user SA token from the handler to the proxy Director.
type SATokenContextKey struct{}

// UserIDContextKey is used to pass the user ID from the handler for 401 retry token refresh.
type UserIDContextKey struct{}

type saTokenContextKey = SATokenContextKey

func (m *MultiClusterProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	clusterID, ok := r.Context().Value(ClusterIDContextKey{}).(string)
	if !ok || clusterID == "" {
		http.Error(w, "missing cluster ID in context", http.StatusBadRequest)
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
		return nil, fmt.Errorf("build proxy for cluster %s: %w", clusterID, err)
	}
	return v.(*httputil.ReverseProxy), nil
}

func (m *MultiClusterProxy) buildProxy(ctx context.Context, clusterID string) (*httputil.ReverseProxy, error) {
	adminKC, err := m.gardener.GetAdminKubeconfig(ctx, clusterID, 0)
	if err != nil {
		return nil, fmt.Errorf("get admin kubeconfig for cluster %s: %w", clusterID, err)
	}

	c, err := NewFromBytes(adminKC.Kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("build client for cluster %s: %w", clusterID, err)
	}

	// Refresh at 70% of the actual expiration time from Gardener.
	ttl := time.Until(adminKC.ExpiresAt)
	refreshAt := time.Now().Add(time.Duration(float64(ttl) * adminKubeconfigRefreshRatio))

	transport := c.Transport()
	if m.tokenCache != nil {
		transport = &retryTransport{
			inner:      transport,
			tokenCache: m.tokenCache,
			logger:     m.logger,
		}
	}

	proxy := buildReverseProxy(c.Host(), transport, m.logger)
	m.proxies.Store(clusterID, &cachedProxy{
		proxy:     proxy,
		expiresAt: refreshAt,
	})
	return proxy, nil
}

func buildReverseProxy(target *url.URL, transport http.RoundTripper, logger *slog.Logger) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Transport: transport,
		// Flush immediately after each write. Go's ReverseProxy auto-detects
		// chunked responses and flushes anyway, but -1 is a defensive default
		// for watch and log-follow streaming.
		FlushInterval: -1,
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
			// Strip client auth headers.
			req.Header.Del("Authorization")
			req.Header.Del("Cookie")
			// Inject per-user SA token if present in context.
			if saToken, ok := req.Context().Value(saTokenContextKey{}).(string); ok && saToken != "" {
				req.Header.Set("Authorization", "Bearer "+saToken)
			}
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			logger.ErrorContext(req.Context(), "kubernetes proxy error", "error", err)
			http.Error(w, "failed to contact kubernetes API", http.StatusBadGateway)
		},
	}
}
