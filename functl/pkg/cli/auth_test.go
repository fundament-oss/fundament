package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyAuthError(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		rejected bool
	}{
		{"unauthenticated", connect.NewError(connect.CodeUnauthenticated, errors.New("bad key")), true},
		{"permission denied", connect.NewError(connect.CodePermissionDenied, errors.New("no")), true},
		{"wrapped", fmt.Errorf("failed to exchange API key for token: %w", connect.NewError(connect.CodeUnauthenticated, errors.New("bad key"))), true},
		{"unavailable", connect.NewError(connect.CodeUnavailable, errors.New("connection refused")), false},
		{"deadline", connect.NewError(connect.CodeDeadlineExceeded, errors.New("timeout")), false},
		{"other", connect.NewError(connect.CodeInternal, errors.New("boom")), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rejected, failure := classifyAuthError(tc.err, "https://authn.example")

			assert.Equal(t, tc.rejected, rejected)
			if tc.rejected {
				require.NoError(t, failure)
				return
			}
			require.ErrorIs(t, failure, tc.err)
		})
	}
}

// statusEnv points functl at authnURL, with apiKey in the environment, and returns
// its config dir.
func statusEnv(t *testing.T, authnURL, apiKey string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("FUNCTL_CONFIG_DIR", dir)
	t.Setenv("FUNDAMENT_API_KEY", apiKey)
	t.Setenv("FUNCTL_AUTHN_URL", authnURL)
	t.Setenv("FUNCTL_API_ENDPOINT", authnURL)
	return dir
}

func runStatus(t *testing.T, output OutputFormat) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	err := (&AuthStatusCmd{}).run(&Context{Output: output}, &buf)
	return buf.String(), err
}

func decode(t *testing.T, out string) map[string]any {
	t.Helper()
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &got), "output: %q", out)
	return got
}

func rejectingServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func closedPort(t *testing.T) string {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	url := "http://" + ln.Addr().String()
	require.NoError(t, ln.Close())
	return url
}

func TestAuthStatus_NotLoggedInIsAnAnswer(t *testing.T) {
	statusEnv(t, "http://127.0.0.1:1", "")

	out, err := runStatus(t, OutputTable)
	require.NoError(t, err)
	assert.Contains(t, out, "functl auth login")

	out, err = runStatus(t, OutputJSON)
	require.NoError(t, err)
	got := decode(t, out)
	assert.Equal(t, false, got["authenticated"])
	assert.Equal(t, authReasonNotLoggedIn, got["reason"])
}

func TestAuthStatus_RejectedKeyIsAnAnswer(t *testing.T) {
	statusEnv(t, rejectingServer(t), "fun_test_key")

	out, err := runStatus(t, OutputTable)
	require.NoError(t, err)
	assert.Contains(t, out, "FUNDAMENT_API_KEY")

	out, err = runStatus(t, OutputJSON)
	require.NoError(t, err)
	got := decode(t, out)
	assert.Equal(t, false, got["authenticated"])
	assert.Equal(t, authReasonRejected, got["reason"])
	assert.Contains(t, got["hint"], "FUNDAMENT_API_KEY")
}

func TestAuthStatus_UnreachableFails(t *testing.T) {
	url := closedPort(t)
	statusEnv(t, url, "fun_test_key")

	for _, output := range []OutputFormat{OutputTable, OutputJSON} {
		out, err := runStatus(t, output)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "could not reach "+url)
		assert.Empty(t, out, "a failed check prints no answer")
	}
}

func TestAuthStatus_UnreadableCredentialsFail(t *testing.T) {
	dir := statusEnv(t, "http://127.0.0.1:1", "")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "credentials"), []byte("api_key: [\n"), 0o600))

	out, err := runStatus(t, OutputJSON)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "credentials file")
	assert.Empty(t, out)
}

func TestAuthStatus_UnreadableConfigFails(t *testing.T) {
	dir := statusEnv(t, "http://127.0.0.1:1", "fun_test_key")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("api_endpoint: [\n"), 0o600))

	out, err := runStatus(t, OutputJSON)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load config")
	assert.Empty(t, out)
}
