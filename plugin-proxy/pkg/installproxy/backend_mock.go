package installproxy

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
)

// MockBackend returns a canned 200 response so the dev iframe can render
// without a live cluster.
type MockBackend struct {
	Logger *slog.Logger
}

func (m MockBackend) Do(r *http.Request, route Route) (*http.Response, error) {
	m.Logger.Debug("mock backend",
		"method", r.Method,
		"kind", route.Kind,
		"cluster", route.ClusterID,
		"install", route.InstallID,
		"plugin", route.PluginName,
		"path", route.RemainingPath,
	)
	body := []byte("mock backend")
	return &http.Response{
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       r,
	}, nil
}
