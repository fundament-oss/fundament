package installproxy

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/fundament-oss/fundament/common/auth"
)

// RouteKind distinguishes the two installation-bound backends.
type RouteKind int

const (
	RouteRuntime RouteKind = iota + 1
	RouteController
)

// Route identifies the backend a request targets.
type Route struct {
	Kind          RouteKind
	ClusterID     string
	InstallID     string
	PluginName    string
	RemainingPath string
}

// Backend forwards an authorized request to the plugin pod / plugin-controller.
type Backend interface {
	Do(r *http.Request, route Route) (*http.Response, error)
}

type BackendFunc func(r *http.Request, route Route) (*http.Response, error)

func (f BackendFunc) Do(r *http.Request, route Route) (*http.Response, error) {
	return f(r, route)
}

// ClusterAuthorizer is the OpenFGA can_view check on (user, cluster).
type ClusterAuthorizer interface {
	CanViewCluster(ctx context.Context, userID, clusterID string) (bool, error)
}

// Handler authenticates and forwards /installations/{id}/{runtime|controller}/*.
type Handler struct {
	jwtSecret []byte
	authz     ClusterAuthorizer
	backend   Backend
	logger    *slog.Logger
}

func New(jwtSecret []byte, authz ClusterAuthorizer, backend Backend, logger *slog.Logger) *Handler {
	return &Handler{jwtSecret: jwtSecret, authz: authz, backend: backend, logger: logger}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. aud=fundament-plugin (signature, expiry).
	const bearerPrefix = "Bearer "
	authz := r.Header.Get("Authorization")
	if !strings.HasPrefix(authz, bearerPrefix) {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return
	}
	bearer := authz[len(bearerPrefix):]
	if bearer == "" {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return
	}
	claims, err := auth.ParsePluginToken(bearer, h.jwtSecret)
	if err != nil {
		h.logger.Debug("plugin token invalid", "err", err)
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	route, ok := parseRoute(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	// 2. URL installation_id must equal the token claim.
	if route.InstallID != claims.InstallationID {
		http.Error(w, "installation_id mismatch", http.StatusForbidden)
		return
	}

	// 3. OpenFGA can_view on (user, cluster) — re-checked per request.
	allowed, err := h.authz.CanViewCluster(r.Context(), claims.Subject, claims.ClusterID)
	if err != nil {
		h.logger.Error("authz check failed", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "permission denied", http.StatusForbidden)
		return
	}

	route.ClusterID = claims.ClusterID
	route.PluginName = claims.PluginName

	resp, err := h.backend.Do(r, route)
	if err != nil {
		h.logger.Warn("backend failed", "err", err, "kind", route.Kind)
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func parseRoute(p string) (Route, bool) {
	const prefix = "/installations/"
	if !strings.HasPrefix(p, prefix) {
		return Route{}, false
	}
	parts := strings.SplitN(strings.TrimPrefix(p, prefix), "/", 3) // id, kind, tail
	if len(parts) < 3 || parts[0] == "" || parts[2] == "" {
		return Route{}, false
	}
	// Reject traversal before the request hits the K8s service-proxy URL.
	// Go's HTTP transport sends paths on the wire as written and does not
	// path-clean, so a "../" segment could escape the /proxy/ scope.
	if strings.Contains(parts[0], "..") || strings.Contains(parts[2], "..") {
		return Route{}, false
	}
	var kind RouteKind
	switch parts[1] {
	case "runtime":
		kind = RouteRuntime
	case "controller":
		kind = RouteController
	default:
		return Route{}, false
	}
	return Route{Kind: kind, InstallID: parts[0], RemainingPath: parts[2]}, true
}
