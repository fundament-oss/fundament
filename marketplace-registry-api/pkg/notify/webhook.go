// Package notify tells reviewers about new review rounds through
// Slack-compatible incoming webhooks: Mattermost, Slack, Rocket.Chat, and Zulip
// or Discord through their Slack endpoints. The payload is {"text": ...}.
//
// NOTE: delivery is best-effort. A message is lost when a webhook is down or
// the pod stops mid-send, and the per-version cooldown lives in memory per
// replica. The planned follow-up is a transactional outbox (like
// tenant.cluster_outbox) written in the submit transaction and drained by a
// worker with retries, which can also hold the cooldown durably.
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

const (
	requestTimeout = 5 * time.Second
	// cooldown suppresses repeat notifications for one version, so a
	// submit → withdraw → submit loop cannot flood the reviewers' channel.
	cooldown = 5 * time.Minute
)

// ReviewRequest is a newly opened review round.
type ReviewRequest struct {
	SubmissionID      uuid.UUID
	PluginVersionID   uuid.UUID
	PluginName        string
	PluginDisplayName string
	Version           string
}

// Webhooks posts to a fixed list of webhook URLs. The list comes from
// deployment config for now; a stored, admin-managed list can replace it.
type Webhooks struct {
	logger   *slog.Logger
	targets  []*url.URL
	adminURL *url.URL
	client   *http.Client
	now      func() time.Time

	mu       sync.Mutex
	notified map[uuid.UUID]time.Time // plugin version → last notification
}

// NewWebhooks validates its URLs up front so a typo fails the deployment, not
// the first submission. Webhook URLs carry a credential, so they must be https,
// and errors and logs only ever name the host. adminURL is required: the link
// is how reviewers act.
func NewWebhooks(logger *slog.Logger, rawURLs []string, adminURL string) (*Webhooks, error) {
	admin, err := parseHTTPS(adminURL)
	if err != nil || admin.RawQuery != "" || admin.Fragment != "" {
		return nil, errors.New("marketplace admin URL must be an absolute https URL without query or fragment when review webhooks are enabled")
	}
	targets := make([]*url.URL, 0, len(rawURLs))
	for i, raw := range rawURLs {
		u, err := parseHTTPS(raw)
		if err != nil {
			return nil, fmt.Errorf("review webhook URL #%d is not an absolute https URL", i+1)
		}
		targets = append(targets, u)
	}
	return &Webhooks{
		logger:   logger,
		targets:  targets,
		adminURL: admin,
		client: &http.Client{
			Timeout: requestTimeout,
			// A redirect can turn the POST into a GET that "succeeds"; report
			// the 3xx as a failure instead.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		now:      time.Now,
		notified: make(map[uuid.UUID]time.Time),
	}, nil
}

// ReviewRequested posts to every webhook and returns once all have finished.
// Failures are logged, never returned: a notification must not fail a submit.
func (w *Webhooks) ReviewRequested(ctx context.Context, req *ReviewRequest) {
	if !w.claim(req.PluginVersionID) {
		w.logger.InfoContext(ctx, "review notification suppressed by cooldown",
			"submission_id", req.SubmissionID, "plugin_version_id", req.PluginVersionID)
		return
	}

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

// claim reports whether versionID may notify now, and if so starts its
// cooldown. Expired entries are pruned on the way.
func (w *Webhooks) claim(versionID uuid.UUID) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := w.now()
	for id, at := range w.notified {
		if now.Sub(at) >= cooldown {
			delete(w.notified, id)
		}
	}
	if _, recent := w.notified[versionID]; recent {
		return false
	}
	w.notified[versionID] = now
	return true
}

func parseHTTPS(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err //nolint:wrapcheck // callers replace it with a message that omits the URL
	}
	if u.Scheme != "https" || u.Host == "" {
		return nil, errors.New("not an absolute https URL")
	}
	return u, nil
}

// message renders publisher-controlled values as code spans, so a crafted
// display name cannot inject links, formatting or mentions. The link is a bare
// URL on its own line: every client auto-links that, while Markdown link
// syntax shows literally in Slack and Google Chat.
func (w *Webhooks) message(req *ReviewRequest) string {
	name := req.PluginDisplayName
	if name == "" {
		name = req.PluginName
	}
	return fmt.Sprintf(":mag: New review requested: %s (%s) version %s\n%s",
		codeSpan(name), codeSpan(req.PluginName), codeSpan(req.Version),
		w.adminURL.JoinPath("admin", "submissions", req.SubmissionID.String()))
}

// codeSpan wraps s in backticks. A backtick or line break would end the span,
// and < > open Slack control sequences (<!channel>, <url|text>) that are not
// documented as inert inside code, so all of them are replaced. Escaping to
// &lt; would show literally in CommonMark clients.
func codeSpan(s string) string {
	s = strings.NewReplacer("`", "'", "\r", " ", "\n", " ", "<", "‹", ">", "›").Replace(s)
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
