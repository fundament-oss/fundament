package proxy

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/gardener"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kube"
)

// allowedPathPrefixes are the Kubernetes API path prefixes the proxy forwards.
// All other paths return 404.
var allowedPathPrefixes = []string{"api", "apis", "openapi/"}

// handleClusterProxy proxies Kubernetes API requests to a specific cluster.
// The cluster ID and remaining path are extracted from the URL via Go 1.22+ wildcards:
//
//	/clusters/{clusterID}/{path...}
//
// Authentication: JWT from Authorization header or fundament_auth cookie.
// Authorization: user must have can_view on the cluster (via OpenFGA).
func (s *Server) handleClusterProxy(w http.ResponseWriter, r *http.Request) {
	// --- Extract cluster ID from path ---

	clusterIDStr := r.PathValue("clusterID")
	clusterID, err := uuid.Parse(clusterIDStr)
	if err != nil {
		http.Error(w, "invalid cluster ID", http.StatusBadRequest)
		return
	}

	// {path...} does not include leading slash.
	path := r.PathValue("path")
	if !isAllowedPath(path) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Rewrite the URL to the forwarded path so downstream handlers (and the
	// public-asset check below) see the canonical Kubernetes API path.
	r.URL.Path = "/" + path
	r.URL.RawPath = ""

	// TODO(FUN-17): plugin console assets now served by plugin-proxy (Plan C);
	// this branch is superseded — remove once Plan C/E land.
	//
	// Plugin console assets are public static UI files. The sandboxed iframe
	// that loads them runs with an opaque origin and cannot send credentials,
	// so the auth/authz check is skipped. The mock handler serves these from
	// disk; in real mode the apiserver service proxy forwards to the plugin
	// pod's HTTP handler (which itself does not authenticate them). The plugin
	// pod sets no CORS/CSP headers, so wrap the writer to stamp them on
	// (see pluginAssetHeaderWriter and kube's plugin_console_assets.go).
	if kube.IsPluginConsoleAssetPath(r.URL.Path) {
		// The asset HTML bootstraps its scripts from the `?host=` origin, so an
		// unrecognized one is refused rather than served: a hand-crafted link must
		// not be able to point a console asset at an attacker's bundle.
		if !s.consoleAssets.AllowsHost(r.URL.Query().Get("host")) {
			http.Error(w, "invalid host origin", http.StatusBadRequest)
			return
		}
		ctx := context.WithValue(r.Context(), kube.ClusterIDContextKey{}, clusterID.String())
		writer := &pluginAssetHeaderWriter{ResponseWriter: w, policy: s.consoleAssets}
		s.kubeHandler.ServeHTTP(writer, r.WithContext(ctx))
		return
	}

	// Dispatch on token type. UserToken is the browser/user path (unchanged).
	// PluginToken is the FUN-17 gateway path: per-request SAR against the user,
	// forward injecting the plugin SA token.
	switch peekTokenType(r) {
	case auth.TokenTypePlugin:
		s.pluginGateway.serve(w, r, clusterID)
	default:
		// UserToken and cookie-borne requests: unchanged behaviour.
		s.handleUserClusterProxy(w, r, clusterID)
	}
}

// handleUserClusterProxy is the unchanged pre-FUN-17 UserToken path.
func (s *Server) handleUserClusterProxy(w http.ResponseWriter, r *http.Request, clusterID uuid.UUID) {
	// --- Authentication ---

	claims, err := s.authValidator.Validate(r.Header)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ctx := WithUserID(r.Context(), claims.UserID())

	// --- Authorization ---

	if err := s.checkPermission(ctx, authz.CanView(), authz.Cluster(clusterID)); err != nil {
		if errors.Is(err, errPermissionDenied) {
			http.Error(w, "permission denied", http.StatusForbidden)
			return
		}
		s.logger.ErrorContext(ctx, "authorization check failed", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// --- Get SA token (real mode only) ---

	if s.tokenCache != nil {
		saToken, err := s.tokenCache.GetToken(ctx, claims.UserID(), clusterID.String())
		if err != nil {
			if errors.Is(err, gardener.ErrSyncPending) {
				http.Error(w, "service account sync pending, try again shortly", http.StatusServiceUnavailable)
				return
			}
			s.logger.ErrorContext(ctx, "failed to get SA token", "error", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		ctx = WithSAToken(ctx, saToken)
	}

	// --- Proxy to Kubernetes API ---

	// Store cluster ID in context for the multi-cluster proxy.
	ctx = context.WithValue(ctx, kube.ClusterIDContextKey{}, clusterID.String())
	r = r.WithContext(ctx)

	s.kubeHandler.ServeHTTP(w, r)
}

// peekTokenType returns the audience-derived token type of the request's
// bearer token (or user for cookie-borne requests), without verifying the
// signature. The real validation happens in the branch handler.
func peekTokenType(r *http.Request) auth.TokenType {
	tok := bearerToken(r)
	if tok == "" {
		// Cookie-borne UserToken: only the browser sends these, never a plugin.
		if c, err := r.Cookie(auth.ConsoleAuthCookieName); err == nil && c.Value != "" {
			return auth.TokenTypeUser
		}
		return ""
	}
	var c auth.Claims
	if _, _, err := new(jwt.Parser).ParseUnverified(tok, &c); err != nil {
		return ""
	}
	if slices.Contains(c.Audience, auth.TokenTypePlugin) {
		return auth.TokenTypePlugin
	}
	return auth.TokenTypeUser
}

// isAllowedPath checks whether the path (without leading slash) starts with
// one of the allowed Kubernetes API prefixes.
func isAllowedPath(path string) bool {
	for _, prefix := range allowedPathPrefixes {
		if path == strings.TrimSuffix(prefix, "/") || strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// pluginAssetHeaderWriter stamps the plugin console asset policy (public CORS + a
// script-src CSP) onto the response. The sandboxed iframe that loads these has an
// opaque origin (Origin: null) the CORS middleware won't allow-list, and the proxied
// plugin pod sets no CORS or CSP headers itself, so the policy is applied just
// before the status line — overriding both the middleware and the proxied response.
//
// Unwrap exposes the underlying writer, so http.ResponseController (which
// httputil.ReverseProxy uses to flush and to set read/write deadlines) reaches the
// real writer's Flusher/Hijacker/deadline support rather than silently getting
// http.ErrNotSupported from the wrapper.
type pluginAssetHeaderWriter struct {
	http.ResponseWriter
	policy  kube.ConsoleAssetPolicy
	applied bool
}

func (w *pluginAssetHeaderWriter) applyHeaders() {
	if w.applied {
		return
	}
	w.applied = true
	w.policy.SetHeaders(w.Header())
}

func (w *pluginAssetHeaderWriter) WriteHeader(status int) {
	w.applyHeaders()
	w.ResponseWriter.WriteHeader(status)
}

func (w *pluginAssetHeaderWriter) Write(b []byte) (int, error) {
	w.applyHeaders()
	return w.ResponseWriter.Write(b)
}

func (w *pluginAssetHeaderWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// Flush forwards to the wrapped writer so the proxy can stream the response. It
// applies the headers first: flushing an uncommitted response makes net/http send
// the header block as it stands, which would otherwise escape without the policy.
func (w *pluginAssetHeaderWriter) Flush() {
	w.applyHeaders()
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
