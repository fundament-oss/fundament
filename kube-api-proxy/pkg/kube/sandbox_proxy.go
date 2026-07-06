package kube

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
)

// SandboxProxy proxies every Kubernetes API request to a single API server
// named by a fixed kubeconfig, regardless of the cluster ID in the request
// context. It's the local-dev shortcut for reaching a k3d plugin sandbox
// cluster without wiring Gardener.
//
// Auth: the underlying Transport handles it via the kubeconfig's admin
// credentials. The Director strips any inbound Authorization (PluginToken,
// UserToken, or the mock SA token set by pluginGateway) — the sandbox
// cluster's API server would reject those anyway; only the kubeconfig's own
// creds are recognized.
type SandboxProxy struct {
	proxy *httputil.ReverseProxy
}

// NewSandboxProxy loads a kubeconfig from disk and returns a proxy pointed at
// its API server.
func NewSandboxProxy(kubeconfigPath string, logger *slog.Logger) (*SandboxProxy, error) {
	data, err := os.ReadFile(kubeconfigPath) //nolint:gosec // path from operator-supplied env var
	if err != nil {
		return nil, fmt.Errorf("read sandbox kubeconfig %q: %w", kubeconfigPath, err)
	}
	c, err := NewFromBytes(data)
	if err != nil {
		return nil, err
	}
	return &SandboxProxy{proxy: buildSandboxReverseProxy(c.Host(), c.Transport(), logger)}, nil
}

func (s *SandboxProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.proxy.ServeHTTP(w, r)
}

func buildSandboxReverseProxy(target *url.URL, transport http.RoundTripper, logger *slog.Logger) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Transport: transport,
		// -1 disables buffering so watch and log-follow requests stream smoothly.
		FlushInterval: -1,
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
			// Strip inbound auth headers — the Transport re-adds the
			// kubeconfig's own bearer/certificate credentials.
			req.Header.Del("Authorization")
			req.Header.Del("Cookie")
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			logger.ErrorContext(req.Context(), "sandbox proxy error", "error", err)
			http.Error(w, "failed to contact sandbox cluster", http.StatusBadGateway)
		},
	}
}
