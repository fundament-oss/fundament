package kube

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
)

// MultiClusterProxy routes Kubernetes API requests to the correct cluster
// based on the Fun-Cluster header. It lazily creates one httputil.ReverseProxy
// per cluster context, caching them for subsequent requests.
//
// The kubeconfig must be a merged kubeconfig where context names equal cluster UUIDs.
type MultiClusterProxy struct {
	kubeconfigPath string
	logger         *slog.Logger
	proxies        sync.Map // string(clusterID) → *httputil.ReverseProxy
}

// NewMultiClusterProxy returns a MultiClusterProxy backed by the merged kubeconfig at path.
func NewMultiClusterProxy(kubeconfigPath string, logger *slog.Logger) *MultiClusterProxy {
	return &MultiClusterProxy{
		kubeconfigPath: kubeconfigPath,
		logger:         logger,
	}
}

func (m *MultiClusterProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Fun-Cluster header is already validated by handler.go before reaching here.
	clusterID := r.Header.Get("Fun-Cluster")
	if clusterID == "" {
		http.Error(w, "missing cluster header", http.StatusBadRequest)
		return
	}

	proxy, err := m.proxyFor(clusterID)
	if err != nil {
		m.logger.ErrorContext(r.Context(), "failed to build proxy for cluster", "cluster", clusterID, "error", err)
		http.Error(w, "failed to contact kubernetes API", http.StatusBadGateway)
		return
	}

	proxy.ServeHTTP(w, r)
}

func (m *MultiClusterProxy) proxyFor(contextName string) (*httputil.ReverseProxy, error) {
	if v, ok := m.proxies.Load(contextName); ok {
		return v.(*httputil.ReverseProxy), nil
	}

	c, err := NewForContext(m.kubeconfigPath, contextName)
	if err != nil {
		return nil, fmt.Errorf("load context %q: %w", contextName, err)
	}

	target := c.Host()
	proxy := buildReverseProxy(target, c.Transport(), m.logger)

	actual, _ := m.proxies.LoadOrStore(contextName, proxy)
	return actual.(*httputil.ReverseProxy), nil
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
