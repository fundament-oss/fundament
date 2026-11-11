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
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/marketplace-registry-api/pkg/notify"
)

const testAdminURL = "https://marketplace-admin.example.com"

// recorder is a TLS webhook that keeps the last body it received.
type recorder struct {
	*httptest.Server
	body  atomic.Value
	calls atomic.Int32
}

func newRecorder(t *testing.T, status int) *recorder {
	t.Helper()
	r := &recorder{}
	r.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		assert.Equal(t, http.MethodPost, req.Method)
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
		b, err := io.ReadAll(req.Body)
		assert.NoError(t, err)
		r.body.Store(b)
		r.calls.Add(1)
		w.WriteHeader(status)
	}))
	t.Cleanup(r.Close)
	return r
}

func (r *recorder) text(t *testing.T) string {
	t.Helper()
	raw, ok := r.body.Load().([]byte)
	require.True(t, ok, "webhook was not called")
	var payload map[string]string
	require.NoError(t, json.Unmarshal(raw, &payload))
	return payload["text"]
}

// newWebhooks trusts the test servers' certificate (all httptest TLS servers
// share one).
func newWebhooks(t *testing.T, logger *slog.Logger, urls []string, adminURL string) *notify.Webhooks {
	t.Helper()
	webhooks, err := notify.NewWebhooks(logger, urls, adminURL)
	require.NoError(t, err)
	ts := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(ts.Close)
	notify.SetTransport(webhooks, ts.Client().Transport)
	return webhooks
}

func request() *notify.ReviewRequest {
	return &notify.ReviewRequest{SubmissionID: uuid.New(), PluginVersionID: uuid.New(), PluginName: "p", Version: "1"}
}

func TestReviewRequestedPostsToEveryWebhook(t *testing.T) {
	a, b := newRecorder(t, http.StatusOK), newRecorder(t, http.StatusOK)
	webhooks := newWebhooks(t, slog.Default(), []string{a.URL + "/hooks/secret", b.URL}, testAdminURL+"/")

	req := &notify.ReviewRequest{
		SubmissionID:      uuid.New(),
		PluginVersionID:   uuid.New(),
		PluginName:        "cert-manager",
		PluginDisplayName: "Cert Manager",
		Version:           "1.2.3",
	}
	webhooks.ReviewRequested(context.Background(), req)

	for _, r := range []*recorder{a, b} {
		assert.Equal(t,
			":mag: New review requested: `Cert Manager` (`cert-manager`) version `1.2.3`\n"+
				testAdminURL+"/admin/submissions/"+req.SubmissionID.String(),
			r.text(t))
	}
}

// The display name is publisher-controlled; it must not break out of its code
// span into links, formatting, mentions or Slack control sequences.
func TestReviewRequestedRendersDisplayNameAsCode(t *testing.T) {
	r := newRecorder(t, http.StatusOK)
	webhooks := newWebhooks(t, slog.Default(), []string{r.URL}, testAdminURL)

	req := request()
	req.PluginDisplayName = "x` [click](https://evil.example) @channel <!channel> <https://evil|Open>\n**urgent**"
	webhooks.ReviewRequested(context.Background(), req)

	assert.Contains(t, r.text(t),
		"`x' [click](https://evil.example) @channel ‹!channel› ‹https://evil|Open› **urgent**` (`p`)")
}

func TestReviewRequestedJoinsTheAdminLinkUnderABasePath(t *testing.T) {
	r := newRecorder(t, http.StatusOK)
	webhooks := newWebhooks(t, slog.Default(), []string{r.URL}, "https://example.com/review/")

	req := request()
	webhooks.ReviewRequested(context.Background(), req)

	assert.Contains(t, r.text(t), "\nhttps://example.com/review/admin/submissions/"+req.SubmissionID.String())
}

func TestReviewRequestedDoesNotFollowRedirects(t *testing.T) {
	target := newRecorder(t, http.StatusOK)
	redirecting := httptest.NewTLSServer(http.RedirectHandler(target.URL, http.StatusFound))
	t.Cleanup(redirecting.Close)

	var logs bytes.Buffer
	webhooks := newWebhooks(t, slog.New(slog.NewTextHandler(&logs, nil)), []string{redirecting.URL}, testAdminURL)

	webhooks.ReviewRequested(context.Background(), request())

	assert.Zero(t, target.calls.Load())
	assert.Contains(t, logs.String(), "unexpected status 302")
}

// A failing webhook must not stop the others, and its log line must not leak
// the secret path of the URL.
func TestReviewRequestedLogsFailuresWithoutTheURL(t *testing.T) {
	ok := newRecorder(t, http.StatusOK)
	failing := newRecorder(t, http.StatusInternalServerError)
	unreachable := httptest.NewTLSServer(http.NotFoundHandler())
	unreachable.Close()

	var logs bytes.Buffer
	webhooks := newWebhooks(t, slog.New(slog.NewTextHandler(&logs, nil)), []string{
		failing.URL + "/hooks/secret-one",
		unreachable.URL + "/hooks/secret-two",
		ok.URL,
	}, testAdminURL)

	webhooks.ReviewRequested(context.Background(), request())

	assert.Equal(t, int32(1), ok.calls.Load())
	assert.Contains(t, logs.String(), "unexpected status 500")
	assert.Equal(t, 2, bytes.Count(logs.Bytes(), []byte("review webhook failed")))
	assert.NotContains(t, logs.String(), "secret-")
}

// A submit → withdraw → submit loop on one version notifies once per cooldown;
// other versions are unaffected.
func TestReviewRequestedCoolsDownPerVersion(t *testing.T) {
	r := newRecorder(t, http.StatusOK)
	webhooks := newWebhooks(t, slog.Default(), []string{r.URL}, testAdminURL)
	now := time.Now()
	notify.SetClock(webhooks, func() time.Time { return now })

	req := request()
	webhooks.ReviewRequested(context.Background(), req)
	webhooks.ReviewRequested(context.Background(), req)
	assert.Equal(t, int32(1), r.calls.Load(), "a repeat within the cooldown is suppressed")

	webhooks.ReviewRequested(context.Background(), request())
	assert.Equal(t, int32(2), r.calls.Load(), "another version still notifies")

	now = now.Add(5 * time.Minute)
	webhooks.ReviewRequested(context.Background(), req)
	assert.Equal(t, int32(3), r.calls.Load(), "the version notifies again after the cooldown")
}

func TestNewWebhooksRejectsInvalidURLs(t *testing.T) {
	// http is rejected too: a webhook URL carries its credential.
	for _, raw := range []string{"", "not a url", "/relative", "ftp://example.com/hook", "http://example.com/hook"} {
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
