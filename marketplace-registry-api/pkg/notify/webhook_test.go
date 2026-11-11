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

const testAdminURL = "https://marketplace-admin.example.com"

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
		assert.Contains(t, payload["text"], "`Cert Manager` (`cert-manager`) version `1.2.3`")
		assert.Contains(t, payload["text"], "https://marketplace-admin.example.com/admin/submissions/"+submissionID.String())
	}
}

// The display name is publisher-controlled; it must not break out of its code
// span into links, formatting or mentions.
func TestReviewRequestedRendersDisplayNameAsCode(t *testing.T) {
	var body atomic.Value
	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body.Store(b)
	}))
	t.Cleanup(ts.Close)

	webhooks, err := notify.NewWebhooks(slog.Default(), []string{ts.URL}, testAdminURL)
	require.NoError(t, err)

	webhooks.ReviewRequested(context.Background(), notify.ReviewRequest{
		SubmissionID:      uuid.New(),
		PluginName:        "p",
		PluginDisplayName: "x` [click](https://evil.example) @channel\n**urgent**",
		Version:           "1",
	})

	raw, ok := body.Load().([]byte)
	require.True(t, ok)
	var payload map[string]string
	require.NoError(t, json.Unmarshal(raw, &payload))
	assert.Contains(t, payload["text"], "`x' [click](https://evil.example) @channel **urgent**` (`p`)")
}

func TestReviewRequestedDoesNotFollowRedirects(t *testing.T) {
	var redirected atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { redirected.Store(true) }))
	t.Cleanup(target.Close)
	redirecting := httptest.NewServer(http.RedirectHandler(target.URL, http.StatusFound))
	t.Cleanup(redirecting.Close)

	var logs bytes.Buffer
	webhooks, err := notify.NewWebhooks(slog.New(slog.NewTextHandler(&logs, nil)), []string{redirecting.URL}, testAdminURL)
	require.NoError(t, err)

	webhooks.ReviewRequested(context.Background(), notify.ReviewRequest{SubmissionID: uuid.New(), PluginName: "p", Version: "1"})

	assert.False(t, redirected.Load())
	assert.Contains(t, logs.String(), "unexpected status 302")
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
	}, testAdminURL)
	require.NoError(t, err)

	webhooks.ReviewRequested(context.Background(), notify.ReviewRequest{SubmissionID: uuid.New(), PluginName: "p", Version: "1"})

	assert.True(t, called.Load())
	assert.Contains(t, logs.String(), "unexpected status 500")
	assert.Equal(t, 2, bytes.Count(logs.Bytes(), []byte("review webhook failed")))
	assert.NotContains(t, logs.String(), "secret-")
}

func TestNewWebhooksRejectsInvalidURLs(t *testing.T) {
	for _, raw := range []string{"", "not a url", "/relative", "ftp://example.com/hook"} {
		_, err := notify.NewWebhooks(slog.Default(), []string{raw}, testAdminURL)
		assert.Error(t, err, "webhook %q", raw)

		_, err = notify.NewWebhooks(slog.Default(), []string{"https://example.com/hook"}, raw)
		assert.Error(t, err, "admin URL %q", raw)
	}

	// The submission path is appended to the admin URL; a query or fragment
	// would swallow it.
	for _, raw := range []string{testAdminURL + "?x=1", testAdminURL + "#top"} {
		_, err := notify.NewWebhooks(slog.Default(), []string{"https://example.com/hook"}, raw)
		assert.Error(t, err, "admin URL %q", raw)
	}
}

func TestReviewRequestedJoinsTheAdminLinkUnderABasePath(t *testing.T) {
	var body atomic.Value
	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body.Store(b)
	}))
	t.Cleanup(ts.Close)

	webhooks, err := notify.NewWebhooks(slog.Default(), []string{ts.URL}, "https://example.com/review/")
	require.NoError(t, err)

	submissionID := uuid.New()
	webhooks.ReviewRequested(context.Background(), notify.ReviewRequest{SubmissionID: submissionID, PluginName: "p", Version: "1"})

	raw, ok := body.Load().([]byte)
	require.True(t, ok)
	assert.Contains(t, string(raw), "(https://example.com/review/admin/submissions/"+submissionID.String()+")")
}
