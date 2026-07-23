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

// allowedPathRoots are the Kubernetes API path roots the proxy forwards,
// matched on the whole first path segment. All other roots return 404.
var allowedPathRoots = []string{"api", "apis", "openapi", "version"}

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

	// {path...} does not include a leading slash.
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
	// Plugin console assets are public static UI files served straight from
	// disk by the in-memory mock client (local dev). The auth/authz check is
	// skipped because those files expose no user-specific data and carry no
	// credentials. serveUnauthedMockAssets is only set for that pure-mock file
	// server (see New): the sandbox and real proxies forward to a live apiserver
	// with admin/SA credentials, so skipping auth there would hand an
	// unauthenticated caller credentialed cluster access. In every other mode
	// this falls through to the normal token/cookie auth path below.
	if s.serveUnauthedMockAssets && kube.IsPluginConsoleAssetPath(r.URL.Path) {
		ctx := context.WithValue(r.Context(), kube.ClusterIDContextKey{}, clusterID.String())
		s.kubeHandler.ServeHTTP(w, r.WithContext(ctx))
		return
	}

	// Dispatch on token type. UserToken is the browser/user path (unchanged).
	// PluginToken is the FUN-17 gateway path: per-request SAR against the user,
	// forward injecting the plugin SA token.
	switch peekTokenType(r) {
	case auth.TokenTypePlugin:
		s.pluginGateway.serve(w, r, clusterID.String())
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

// isAllowedPath reports whether the first segment of the raw wildcard path (the
// {path...} value, without a leading slash) is an allowed Kubernetes API root.
// It matches whole segments, so "apix" or "versionz" do not match "api" /
// "version". It does not concern itself with "." / ".." segments: the apiserver
// owns path normalization and authorizes every request against the injected SA
// token, so a traversal attempt cannot cross a privilege boundary here.
func isAllowedPath(rawPath string) bool {
	root, _, _ := strings.Cut(rawPath, "/")
	return slices.Contains(allowedPathRoots, root)
}
