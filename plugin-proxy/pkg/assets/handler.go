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
)

// Fetcher loads one asset file (body, content-type) from a backing source.
// The handler pre-resolves clusterID so the implementation can stay thin
// (PodFetcher just builds a service-proxy URL).
type Fetcher interface {
	Fetch(ctx context.Context, clusterID uuid.UUID, pluginName, assetPath string) ([]byte, string, error)
}

// ClusterResolver maps (pluginName, version) to the cluster currently running
// that version. Asset bundles are content-addressed by (pluginName, version),
// so any cluster running the pair serves the same bytes.
type ClusterResolver interface {
	ClusterFor(ctx context.Context, pluginName, version string) (uuid.UUID, error)
}

// CSPConfig holds the dynamic origins of the FUN-17 plugin CSP.
type CSPConfig struct {
	// ConnectSrc / FormAction: the two proxy origins the plugin JS may reach.
	ConnectSrc []string
	FormAction []string
	// FrameAncestors: origins allowed to embed the iframe (the Console).
	FrameAncestors []string
}

type handler struct {
	resolver ClusterResolver
	fetcher  Fetcher
	csp      string
	logger   *slog.Logger
}

func NewHandler(resolver ClusterResolver, fetcher Fetcher, csp *CSPConfig, logger *slog.Logger) http.Handler {
	return &handler{resolver: resolver, fetcher: fetcher, csp: buildCSP(csp), logger: logger}
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

// ServeHTTP serves /plugins/{name}/{version}/console/{path...}.
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name, version, asset, ok := parsePath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	clusterID, err := h.resolver.ClusterFor(r.Context(), name, version)
	if err != nil {
		h.logger.Warn("cluster resolve failed", "plugin", name, "version", version, "err", err)
		http.Error(w, "asset unavailable", http.StatusBadGateway)
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
	_, _ = w.Write(body)
}

func parsePath(p string) (name, version, asset string, ok bool) {
	const prefix = "/plugins/"
	if !strings.HasPrefix(p, prefix) {
		return "", "", "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(p, prefix), "/", 4) // name, version, "console", path
	if len(parts) < 4 || parts[2] != "console" {
		return "", "", "", false
	}
	name, version, asset = parts[0], parts[1], parts[3]
	if name == "" || version == "" || asset == "" {
		return "", "", "", false
	}
	// Check the raw asset for traversal before path.Clean normalises it away.
	if strings.Contains(asset, "..") {
		return "", "", "", false
	}
	cleaned := path.Clean("/" + asset)
	if cleaned == "/" {
		return "", "", "", false
	}
	return name, version, strings.TrimPrefix(cleaned, "/"), true
}

func guessContentType(assetPath string) string {
	if ct := mime.TypeByExtension(strings.ToLower(path.Ext(assetPath))); ct != "" {
		return ct
	}
	return "application/octet-stream"
}
