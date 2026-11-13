package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
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
	t.Setenv("FUNCTL_CONFIG_DIR", t.TempDir())
	t.Setenv("FUNDAMENT_API_KEY", "fun_test_key")
	t.Setenv("FUNCTL_AUTHN_URL", authnURL)
	t.Setenv("FUNCTL_API_ENDPOINT", authnURL)
	return (&AuthStatusCmd{}).Run(&Context{Output: OutputTable})
}

func TestAuthStatus_RejectedKeyFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	err := runStatus(t, srv.URL)

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
