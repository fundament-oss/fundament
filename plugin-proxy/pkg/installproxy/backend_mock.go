package installproxy

import (
	"log/slog"
	"net/http"
)

// MockBackend returns a canned 200 response so the dev iframe can render
// without a live cluster.
type MockBackend struct {
	Logger *slog.Logger
}

func (m MockBackend) Serve(w http.ResponseWriter, r *http.Request, route Route) {
	m.Logger.Debug("mock backend",
		"method", r.Method,
		"kind", route.Kind,
		"cluster", route.ClusterID,
		"install", route.InstallID,
		"plugin", route.PluginName,
		"path", route.RemainingPath,
	)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("mock backend"))
}
