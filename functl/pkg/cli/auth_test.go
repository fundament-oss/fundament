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
		name   string
		err    error
		reason string
	}{
		{"unauthenticated", connect.NewError(connect.CodeUnauthenticated, errors.New("bad key")), authReasonRejected},
		{"permission denied", connect.NewError(connect.CodePermissionDenied, errors.New("no")), authReasonRejected},
		{"unavailable", connect.NewError(connect.CodeUnavailable, errors.New("connection refused")), authReasonUnreachable},
		{"deadline", connect.NewError(connect.CodeDeadlineExceeded, errors.New("timeout")), authReasonUnreachable},
		{"wrapped", fmt.Errorf("failed to exchange API key for token: %w", connect.NewError(connect.CodeUnauthenticated, errors.New("bad key"))), authReasonRejected},
		{"other", connect.NewError(connect.CodeInternal, errors.New("boom")), authReasonError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reason, err := classifyAuthError(tc.err, "https://authn.example")
			assert.Equal(t, tc.reason, reason)
			require.ErrorIs(t, err, tc.err)
			if reason == authReasonUnreachable {
				assert.Contains(t, err.Error(), "https://authn.example")
			}
		})
	}
}

func TestReportNotAuthenticated_JSON(t *testing.T) {
	var buf bytes.Buffer
	failure := errors.New("not authenticated")

	err := reportNotAuthenticated(&buf, OutputJSON, authReasonNotLoggedIn, failure)

	require.ErrorIs(t, err, failure)
	var got map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	assert.Equal(t, map[string]any{
		"authenticated": false,
		"reason":        authReasonNotLoggedIn,
		"message":       "not authenticated",
	}, got)
}

func TestReportNotAuthenticated_Table(t *testing.T) {
	var buf bytes.Buffer
	failure := errors.New("not authenticated")

	err := reportNotAuthenticated(&buf, OutputTable, authReasonNotLoggedIn, failure)

	require.ErrorIs(t, err, failure)
	assert.Empty(t, buf.String())
}

func TestAuthStatus_NotLoggedInFails(t *testing.T) {
	t.Setenv("FUNCTL_CONFIG_DIR", t.TempDir())
	t.Setenv("FUNDAMENT_API_KEY", "")

	err := (&AuthStatusCmd{}).Run(&Context{Output: OutputTable})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "functl auth login")
}

// runStatus runs `auth status` with an API key from the environment against authnURL.
func runStatus(t *testing.T, authnURL string) error {
	t.Helper()
	setStatusEnv(t, authnURL)
	return (&AuthStatusCmd{}).Run(&Context{Output: OutputTable})
}

// setStatusEnv points functl at authnURL with an API key and returns its config dir.
func setStatusEnv(t *testing.T, authnURL string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("FUNCTL_CONFIG_DIR", dir)
	t.Setenv("FUNDAMENT_API_KEY", "fun_test_key")
	t.Setenv("FUNCTL_AUTHN_URL", authnURL)
	t.Setenv("FUNCTL_API_ENDPOINT", authnURL)
	return dir
}

// runStatusJSON runs `auth status -o json` and returns the decoded answer.
func runStatusJSON(t *testing.T) (map[string]any, error) {
	t.Helper()
	var buf bytes.Buffer
	err := (&AuthStatusCmd{}).run(&Context{Output: OutputJSON}, &buf)
	var got map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got), "output: %q", buf.String())
	return got, err
}

func rejectingServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestAuthStatus_JSONNotLoggedIn(t *testing.T) {
	t.Setenv("FUNCTL_CONFIG_DIR", t.TempDir())
	t.Setenv("FUNDAMENT_API_KEY", "")

	got, err := runStatusJSON(t)

	require.Error(t, err)
	assert.Equal(t, false, got["authenticated"])
	assert.Equal(t, authReasonNotLoggedIn, got["reason"])
}

func TestAuthStatus_JSONRejected(t *testing.T) {
	setStatusEnv(t, rejectingServer(t))

	got, err := runStatusJSON(t)

	require.Error(t, err)
	assert.Equal(t, false, got["authenticated"])
	assert.Equal(t, authReasonRejected, got["reason"])
	assert.Contains(t, got["message"], "FUNDAMENT_API_KEY")
}

func TestAuthStatus_JSONBrokenConfig(t *testing.T) {
	dir := setStatusEnv(t, "http://127.0.0.1:1")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("api_endpoint: [\n"), 0o600))

	got, err := runStatusJSON(t)

	require.Error(t, err)
	assert.Equal(t, false, got["authenticated"])
	assert.Equal(t, authReasonError, got["reason"])
	assert.Contains(t, got["message"], "failed to load config")
}

func TestAuthStatus_RejectedKeyFails(t *testing.T) {
	err := runStatus(t, rejectingServer(t))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "credentials rejected")
}

func TestAuthStatus_UnreachableFails(t *testing.T) {
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	url := "http://" + ln.Addr().String()
	require.NoError(t, ln.Close())

	err = runStatus(t, url)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not reach "+url)
}
