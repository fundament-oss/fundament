package proxy

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kube"
	tokenpkg "github.com/fundament-oss/fundament/kube-api-proxy/pkg/token"
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
			if errors.Is(err, tokenpkg.ErrSyncPending) {
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

	// Rewrite the URL to the forwarded path (prepend leading slash).
	r.URL.Path = "/" + path
	r.URL.RawPath = ""

	// Store cluster ID in context for the multi-cluster proxy.
	ctx = context.WithValue(ctx, kube.ClusterIDContextKey{}, clusterID.String())
	r = r.WithContext(ctx)

	s.kubeHandler.ServeHTTP(w, r)
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
