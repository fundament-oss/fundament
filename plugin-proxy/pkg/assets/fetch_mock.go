package assets

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
)

// MockFetcher serves the same minimal HTML body for every asset request so
// the dev iframe renders without a live cluster.
type MockFetcher struct {
	Logger *slog.Logger
}

func (m MockFetcher) Fetch(_ context.Context, clusterID uuid.UUID, pluginName, pluginVersion, assetPath string) ([]byte, string, error) {
	m.Logger.Debug("mock asset fetch", "cluster", clusterID, "plugin", pluginName, "version", pluginVersion, "path", assetPath)
	body := []byte("<!doctype html><html><body>mock asset</body></html>")
	return body, "text/html; charset=utf-8", nil
}
