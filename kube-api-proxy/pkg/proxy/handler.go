package proxy

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/common/authz"
)

// OrganizationHeader is the header name for selecting the active organization.
const OrganizationHeader = "Fun-Organization"

// ClusterHeader is the header name for selecting the target cluster.
const ClusterHeader = "Fun-Cluster"

// handleClusterProxy is a read-only HTTP proxy to the Kubernetes API for a specific cluster.
// Path format: /k8s-api/{...kubernetes_api_path}
//
// Authentication: JWT from Authorization header or fundament_auth cookie,
// plus Fun-Organization header for org scoping.
// Authorization: user must have can_view on the cluster (via OpenFGA).
func (s *Server) handleClusterProxy(w http.ResponseWriter, r *http.Request) {
	// Read-only: reject anything that is not a GET request.
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// --- Authentication ---

	claims, err := s.authValidator.Validate(r.Header)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	orgHeader := r.Header.Get(OrganizationHeader)
	if orgHeader == "" {
		http.Error(w, fmt.Sprintf("missing %s header", OrganizationHeader), http.StatusBadRequest)
		return
	}

	organizationID, err := uuid.Parse(orgHeader)
	if err != nil {
		http.Error(w, "invalid organization ID", http.StatusBadRequest)
		return
	}

	if !slices.Contains(claims.OrganizationIDs, organizationID) {
		http.Error(w, "permission denied", http.StatusForbidden)
		return
	}

	ctx := WithUserID(r.Context(), claims.UserID())

	// --- Read cluster ID from header ---
	clusterHeader := r.Header.Get(ClusterHeader)
	if clusterHeader == "" {
		http.Error(w, fmt.Sprintf("missing %s header", ClusterHeader), http.StatusBadRequest)
		return
	}

	clusterID, err := uuid.Parse(clusterHeader)
	if err != nil {
		http.Error(w, "invalid cluster ID", http.StatusBadRequest)
		return
	}

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

	// --- Proxy to Kubernetes API ---

	k8sPath := strings.TrimPrefix(r.URL.Path, "/k8s-api")

	// Only allow standard Kubernetes API paths to prevent SSRF.
	if !strings.HasPrefix(k8sPath, "/apis/") && !strings.HasPrefix(k8sPath, "/api/") {
		http.Error(w, "forbidden path", http.StatusForbidden)
		return
	}

	r.URL.Path = k8sPath
	r.URL.RawPath = ""
	s.kubeHandler.ServeHTTP(w, r)
}
