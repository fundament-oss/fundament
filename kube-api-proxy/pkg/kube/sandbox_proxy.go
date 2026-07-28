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
// Auth: two modes.
//   - Plugin requests: pluginGateway stores the plugin SA token on the request
//     context via WithSAToken; the Director reads it and sets Authorization
//     Bearer <SA-token>. The sandbox cluster verifies against the plugin SA's
//     RBAC — this is what the FUN-17 plugin-scope ClusterRole is meant to
//     enforce, and it MUST be exercised in local dev too.
//   - Non-plugin requests (no SA token on ctx): the kubeconfig's transport
//     supplies admin credentials. This preserves the existing behaviour for
//     UserToken paths that hit the sandbox proxy directly.
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
			// Strip inbound Cookie so the console's UserToken never leaks
			// upstream. Authorization is set below.
			req.Header.Del("Cookie")

			// If pluginGateway put an SA token on the ctx (plugin path), use
			// it — the sandbox cluster must see the plugin SA identity so the
			// FUN-17 plugin-scope ClusterRole gates apply the same way as in
			// prod. Otherwise let the Transport attach the kubeconfig's admin
			// creds (non-plugin paths, e.g. UserToken flows).
			saToken, ok := req.Context().Value(SATokenContextKey{}).(string)
			if ok && saToken != "" {
				req.Header.Set("Authorization", "Bearer "+saToken)
			} else {
				req.Header.Del("Authorization")
			}
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			logger.ErrorContext(req.Context(), "sandbox proxy error", "error", err)
			http.Error(w, "failed to contact sandbox cluster", http.StatusBadGateway)
		},
	}
}
