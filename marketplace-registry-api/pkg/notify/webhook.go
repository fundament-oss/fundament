// Package notify tells reviewers about new review rounds through incoming
// webhooks (Mattermost, or anything accepting the same {"text": ...} payload).
//
// NOTE: delivery is best-effort. A message is lost when a webhook is down or
// the pod stops mid-send. The planned follow-up is a transactional outbox
// (like tenant.cluster_outbox) written in the submit transaction and drained
// by a worker with retries.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const requestTimeout = 5 * time.Second

// ReviewRequest is a newly opened review round.
type ReviewRequest struct {
	SubmissionID      uuid.UUID
	PluginName        string
	PluginDisplayName string
	Version           string
}

// Webhooks posts to a fixed list of webhook URLs. The list comes from
// deployment config for now; a stored, admin-managed list can replace it.
type Webhooks struct {
	logger   *slog.Logger
	targets  []*url.URL
	adminURL string
	client   *http.Client
}

// NewWebhooks validates its URLs up front so a typo fails the deployment, not
// the first submission. Webhook URLs embed a secret, so errors and logs only
// ever name the host. adminURL is required: the link is how reviewers act.
func NewWebhooks(logger *slog.Logger, rawURLs []string, adminURL string) (*Webhooks, error) {
	if !isAbsoluteHTTP(adminURL) {
		return nil, errors.New("marketplace admin URL must be an absolute http(s) URL when review webhooks are enabled")
	}
	targets := make([]*url.URL, 0, len(rawURLs))
	for i, raw := range rawURLs {
		raw = strings.TrimSpace(raw)
		if !isAbsoluteHTTP(raw) {
			return nil, fmt.Errorf("review webhook URL #%d is not an absolute http(s) URL", i+1)
		}
		u, _ := url.Parse(raw)
		targets = append(targets, u)
	}
	return &Webhooks{
		logger:   logger,
		targets:  targets,
		adminURL: strings.TrimRight(adminURL, "/"),
		client: &http.Client{
			Timeout: requestTimeout,
			// A redirect can turn the POST into a GET that "succeeds"; report
			// the 3xx as a failure instead.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

// ReviewRequested posts to every webhook and returns once all have finished.
// Failures are logged, never returned: a notification must not fail a submit.
func (w *Webhooks) ReviewRequested(ctx context.Context, req ReviewRequest) {
	body, err := json.Marshal(map[string]string{"text": w.message(req)})
	if err != nil {
		w.logger.ErrorContext(ctx, "encoding review webhook payload", "error", err)
		return
	}

	var wg sync.WaitGroup
	for _, target := range w.targets {
		wg.Go(func() {
			if err := w.post(ctx, target, body); err != nil {
				w.logger.WarnContext(ctx, "review webhook failed",
					"host", target.Host, "submission_id", req.SubmissionID, "error", err)
			}
		})
	}
	wg.Wait()
}

func isAbsoluteHTTP(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}

// message renders publisher-controlled values as code spans, so a crafted
// display name cannot inject links, formatting or @channel mentions.
func (w *Webhooks) message(req ReviewRequest) string {
	name := req.PluginDisplayName
	if name == "" {
		name = req.PluginName
	}
	return fmt.Sprintf(":mag: New review requested: %s (%s) version %s\n[Open submission](%s/admin/submissions/%s)",
		codeSpan(name), codeSpan(req.PluginName), codeSpan(req.Version), w.adminURL, req.SubmissionID)
}

// codeSpan wraps s in backticks. Code spans cannot contain a backtick or a
// line break, so those are replaced rather than escaped.
func codeSpan(s string) string {
	s = strings.NewReplacer("`", "'", "\r", " ", "\n", " ").Replace(s)
	return "`" + s + "`"
}

func (w *Webhooks) post(ctx context.Context, target *url.URL, body []byte) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return redact(err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := w.client.Do(httpReq)
	if err != nil {
		return redact(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

// redact drops the *url.Error wrapper, whose message includes the full,
// secret-bearing URL.
func redact(err error) error {
	if urlErr, ok := errors.AsType[*url.Error](err); ok {
		return urlErr.Err
	}
	return err
}
