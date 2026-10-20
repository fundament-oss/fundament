package health

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
func TestReadyzKeepsDependencyErrorOutOfTheResponse(t *testing.T) {
	pool, err := pgxpool.New(context.Background(),
		"postgres://health_probe_role@127.0.0.1:1/health_probe_db?sslmode=disable&connect_timeout=1")
	require.NoError(t, err)
	defer pool.Close()

	var logged bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logged, nil))

	recorder := httptest.NewRecorder()
	handler := Readyz(logger, Check{Name: "database", Probe: pool.Ping})
	handler(recorder, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/readyz", http.NoBody))

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Equal(t, "database unavailable", recorder.Body.String())
	assert.NotContains(t, recorder.Body.String(), "health_probe_db")
	assert.NotContains(t, recorder.Body.String(), "health_probe_role")
	assert.Contains(t, logged.String(), "health_probe_db", "the detail must still reach the log")
}

// Only the failed dependencies are named; a passing check stays silent and
// all checks run even when an earlier one fails.
func TestReadyzNamesEveryFailedCheck(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	ok := func(context.Context) error { return nil }
	fail := func(context.Context) error { return assert.AnError }

	recorder := httptest.NewRecorder()
	Readyz(logger,
		Check{Name: "database", Probe: ok},
		Check{Name: "openfga", Probe: fail},
		Check{Name: "authz_outbox", Probe: fail},
	)(recorder, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/readyz", http.NoBody))

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Equal(t, "openfga unavailable\nauthz_outbox unavailable", recorder.Body.String())
}

func TestReadyzAllPassing(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))

	recorder := httptest.NewRecorder()
	Readyz(logger, Check{Name: "database", Probe: func(context.Context) error { return nil }})(
		recorder, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/readyz", http.NoBody))

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "ok", recorder.Body.String())
}
