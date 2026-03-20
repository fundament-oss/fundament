package proxy

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/common/authz"
)

// OrganizationHeader is the header name for selecting the active organization.
const OrganizationHeader = "Fun-Organization"

// handleClusterProxy is a read-only HTTP proxy to the Kubernetes API for a specific cluster.
// Path format: /k8sproxy/{clusterID}/{...kubernetes_api_path}
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

	// --- Parse cluster ID from URL ---
	// Path: /k8sproxy/{clusterID}/{...}
	rest := strings.TrimPrefix(r.URL.Path, "/k8sproxy/")
	clusterIDStr, k8sPath, _ := strings.Cut(rest, "/")
	if clusterIDStr == "" {
		http.Error(w, "missing cluster ID in path", http.StatusBadRequest)
		return
	}

	clusterID, err := uuid.Parse(clusterIDStr)
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

	k8sPath = "/" + k8sPath

	// Only allow standard Kubernetes API paths to prevent SSRF.
	if !strings.HasPrefix(k8sPath, "/apis/") && !strings.HasPrefix(k8sPath, "/api/") {
		http.Error(w, "forbidden path", http.StatusForbidden)
		return
	}

	if s.kubeProxy != nil {
		// Real mode: let httputil.ReverseProxy forward to the K8s API.
		// It handles response headers, streaming, and transport-level auth.
		r.URL.Path = k8sPath
		r.URL.RawPath = ""
		s.kubeProxy.ServeHTTP(w, r)
		return
	}

	// Mock mode: use the in-process mock client.
	if r.URL.RawQuery != "" {
		k8sPath = k8sPath + "?" + r.URL.RawQuery
	}

	statusCode, body, err := s.kubeClient.Do(ctx, r.Method, k8sPath, r.Body)
	if err != nil {
		s.logger.ErrorContext(ctx, "kubernetes client error", "error", err, "path", k8sPath)
		http.Error(w, "failed to contact kubernetes API", http.StatusBadGateway)
		return
	}
	defer body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = io.Copy(w, body)
}
