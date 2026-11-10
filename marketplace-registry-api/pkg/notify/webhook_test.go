package notify_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/marketplace-registry-api/pkg/notify"
)

func TestReviewRequestedPostsToEveryWebhook(t *testing.T) {
	var bodies [2]atomic.Value
	servers := make([]string, 2)
	for i := range servers {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			b, err := io.ReadAll(r.Body)
			assert.NoError(t, err)
			bodies[i].Store(b)
		}))
		t.Cleanup(ts.Close)
		servers[i] = ts.URL + "/hooks/secret"
	}

	webhooks, err := notify.NewWebhooks(slog.Default(), servers, "https://marketplace-admin.example.com/")
	require.NoError(t, err)

	submissionID := uuid.New()
	webhooks.ReviewRequested(context.Background(), notify.ReviewRequest{
		SubmissionID:      submissionID,
		PluginName:        "cert-manager",
		PluginDisplayName: "Cert Manager",
		Version:           "1.2.3",
	})

	for i := range bodies {
		raw, ok := bodies[i].Load().([]byte)
		require.True(t, ok, "webhook %d was not called", i)

		var payload map[string]string
		require.NoError(t, json.Unmarshal(raw, &payload))
		assert.Contains(t, payload["text"], "**Cert Manager** (`cert-manager`) version `1.2.3`")
		assert.Contains(t, payload["text"], "https://marketplace-admin.example.com/admin/submissions/"+submissionID.String())
	}
}

func TestReviewRequestedOmitsLinkWithoutAdminURL(t *testing.T) {
	var body atomic.Value
	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body.Store(b)
	}))
	t.Cleanup(ts.Close)

	webhooks, err := notify.NewWebhooks(slog.Default(), []string{ts.URL}, "")
	require.NoError(t, err)

	webhooks.ReviewRequested(context.Background(), notify.ReviewRequest{SubmissionID: uuid.New(), PluginName: "p", Version: "1"})

	raw, ok := body.Load().([]byte)
	require.True(t, ok)
	assert.NotContains(t, string(raw), "Open submission")
	assert.Contains(t, string(raw), "**p** (`p`)")
}

// A failing webhook must not stop the others, and its log line must not leak
// the secret path of the URL.
func TestReviewRequestedLogsFailuresWithoutTheURL(t *testing.T) {
	var called atomic.Bool
	ok := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called.Store(true) }))
	t.Cleanup(ok.Close)
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(failing.Close)
	unreachable := httptest.NewServer(http.NotFoundHandler())
	unreachable.Close()

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	webhooks, err := notify.NewWebhooks(logger, []string{
		failing.URL + "/hooks/secret-one",
		unreachable.URL + "/hooks/secret-two",
		ok.URL,
	}, "")
	require.NoError(t, err)

	webhooks.ReviewRequested(context.Background(), notify.ReviewRequest{SubmissionID: uuid.New(), PluginName: "p", Version: "1"})

	assert.True(t, called.Load())
	assert.Contains(t, logs.String(), "unexpected status 500")
	assert.Equal(t, 2, bytes.Count(logs.Bytes(), []byte("review webhook failed")))
	assert.NotContains(t, logs.String(), "secret-")
}

func TestNewWebhooksRejectsInvalidURLs(t *testing.T) {
	for _, raw := range []string{"", "not a url", "/relative", "ftp://example.com/hook"} {
		_, err := notify.NewWebhooks(slog.Default(), []string{raw}, "")
		assert.Error(t, err, "%q", raw)
	}
}
