package health

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The probes gate the rollout: if livez or readyz stop answering 200 the
// Deployment never goes ready.
func TestMuxServesProbes(t *testing.T) {
	server := httptest.NewServer(NewMux(slog.New(slog.DiscardHandler), "v1.2.3", nil))
	t.Cleanup(server.Close)

	for _, path := range []string{"/livez", "/readyz"} {
		t.Run(path, func(t *testing.T) {
			resp, err := server.Client().Get(server.URL + path)
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}

// /version is how CI tells which release is answering, so it must echo
// DEPLOYMENT_VERSION verbatim.
func TestMuxVersionEchoesDeploymentVersion(t *testing.T) {
	server := httptest.NewServer(NewMux(slog.New(slog.DiscardHandler), "v1.2.3", nil))
	t.Cleanup(server.Close)

	resp, err := server.Client().Get(server.URL + "/version")
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "v1.2.3", string(body))
}
