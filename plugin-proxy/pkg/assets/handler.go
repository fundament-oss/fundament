package assets

import (
	"context"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/common/auth"
)

// Fetcher loads one asset file (body, content-type) from a backing source.
// The clusterID names the target cluster (from the request URL); PodFetcher
// resolves it to a kubeconfig and builds a service-proxy URL against that
// cluster's API.
type Fetcher interface {
	Fetch(ctx context.Context, clusterID uuid.UUID, pluginName, assetPath string) ([]byte, string, error)
}

// CSPConfig holds the dynamic origins of the FUN-17 plugin CSP.
type CSPConfig struct {
	// ConnectSrc / FormAction: the two proxy origins the plugin JS may reach.
	ConnectSrc []string
	FormAction []string
	// FrameAncestors: origins allowed to embed the iframe (the Console).
	FrameAncestors []string
}

// ClusterViewChecker gates asset requests on the caller's OpenFGA
// can_view(user, cluster) — same check authn-api runs before minting a
// PluginToken. Cookie-based auth (browser navigation can't attach a Bearer
// token to an iframe src or <script>/<link> subresources).
type ClusterViewChecker interface {
	CanViewCluster(ctx context.Context, userID, clusterID uuid.UUID) (bool, error)
}

type handler struct {
	fetcher   Fetcher
	csp       string
	logger    *slog.Logger
	validator *auth.Validator
	canView   ClusterViewChecker
}

// NewHandler wires the asset handler. `validator` parses the UserToken cookie
// (`fundament_auth`); `canView` calls OpenFGA to authorize the user on the
// requested cluster.
func NewHandler(fetcher Fetcher, csp *CSPConfig, validator *auth.Validator, canView ClusterViewChecker, logger *slog.Logger) http.Handler {
	return &handler{
		fetcher:   fetcher,
		csp:       buildCSP(csp),
		validator: validator,
		canView:   canView,
		logger:    logger,
	}
}

// buildCSP produces exactly the FUN-17 plugin Content-Security-Policy.
// Note: NO 'unsafe-inline' — the SDK and plugin scripts ship as separate files.
func buildCSP(c *CSPConfig) string {
	join := func(v []string) string {
		if len(v) == 0 {
			return "'self'"
		}
		return strings.Join(v, " ")
	}
	return strings.Join([]string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self'",
		fmt.Sprintf("connect-src %s", join(c.ConnectSrc)),
		fmt.Sprintf("form-action %s", join(c.FormAction)),
		fmt.Sprintf("frame-ancestors %s", join(c.FrameAncestors)),
		"base-uri 'none'",
		"object-src 'none'",
	}, "; ")
}

// ServeHTTP serves /clusters/{clusterID}/plugins/{name}/{version}/console/{path...}.
//
// The cluster is chosen by the caller (the console picks the cluster the user
// is browsing) so asset traffic lands on the same cluster the plugin will
// actually operate against. This keeps load local to the user's cluster and
// avoids one cluster becoming the asset proxy for every plugin installation
// across the estate.
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clusterID, name, _, asset, ok := parsePath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	// Auth: parse the console's UserToken cookie, then OpenFGA-check that the
	// user can view this cluster. Unauthenticated and unauthorized collapse to
	// 404 so the endpoint doesn't leak which (cluster, plugin, version) tuples
	// are valid.
	claims, err := h.validator.Validate(r.Header)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	userID := claims.UserID()
	if userID == uuid.Nil {
		http.NotFound(w, r)
		return
	}

	allowed, err := h.canView.CanViewCluster(r.Context(), userID, clusterID)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "can_view check failed", "user", userID, "cluster", clusterID, "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if !allowed {
		http.NotFound(w, r)
		return
	}

	body, ct, err := h.fetcher.Fetch(r.Context(), clusterID, name, asset)
	if err != nil {
		h.logger.Warn("asset fetch failed", "cluster", clusterID, "plugin", name, "path", asset, "err", err)
		http.Error(w, "asset unavailable", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Security-Policy", h.csp)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")

	if r.Method == http.MethodHead {
		return
	}

	//nolint:gosec // intentional asset proxy: Content-Type is set from upstream, X-Content-Type-Options=nosniff and a strict CSP confine execution.
	_, _ = w.Write(body)
}

func parsePath(p string) (clusterID uuid.UUID, name, version, asset string, ok bool) {
	const prefix = "/clusters/"
	if !strings.HasPrefix(p, prefix) {
		return uuid.Nil, "", "", "", false
	}

	// clusters/{id}/plugins/{name}/{version}/console/{path...}
	parts := strings.SplitN(strings.TrimPrefix(p, prefix), "/", 6)
	if len(parts) < 6 || parts[1] != "plugins" || parts[4] != "console" {
		return uuid.Nil, "", "", "", false
	}

	rawCluster, name, version, asset := parts[0], parts[2], parts[3], parts[5]
	if rawCluster == "" || name == "" || version == "" || asset == "" {
		return uuid.Nil, "", "", "", false
	}

	clusterID, err := uuid.Parse(rawCluster)
	if err != nil {
		return uuid.Nil, "", "", "", false
	}

	// Check the raw asset for traversal before path.Clean normalises it away.
	if strings.Contains(asset, "..") {
		return uuid.Nil, "", "", "", false
	}

	cleaned := path.Clean("/" + asset)
	if cleaned == "/" {
		return uuid.Nil, "", "", "", false
	}

	return clusterID, name, version, strings.TrimPrefix(cleaned, "/"), true
}

func guessContentType(assetPath string) string {
	if ct := mime.TypeByExtension(strings.ToLower(path.Ext(assetPath))); ct != "" {
		return ct
	}

	return "application/octet-stream"
}
