package main

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// /readyz is served on the outer mux, which the Ingress exposes with nothing
// authenticating in front of it. A pgx error names the host, port, database and
// role, so the caller gets a fixed string and the detail goes to the log.
func TestReadyzKeepsDatabaseErrorOutOfTheResponse(t *testing.T) {
	pool, err := pgxpool.New(context.Background(),
		"postgres://catalog_probe_role@127.0.0.1:1/catalog_probe_db?sslmode=disable&connect_timeout=1")
	require.NoError(t, err)
	defer pool.Close()

	var logged bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logged, nil))

	recorder := httptest.NewRecorder()
	readyz(logger, pool)(recorder, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/readyz", http.NoBody))

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Equal(t, "database unavailable", recorder.Body.String())
	assert.NotContains(t, recorder.Body.String(), "catalog_probe_db")
	assert.NotContains(t, recorder.Body.String(), "catalog_probe_role")
	assert.Contains(t, logged.String(), "catalog_probe_db", "the detail must still reach the log")
}
