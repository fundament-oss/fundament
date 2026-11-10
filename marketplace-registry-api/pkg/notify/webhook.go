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

// NewWebhooks validates rawURLs up front so a typo fails the deployment, not
// the first submission. Webhook URLs embed a secret, so errors and logs only
// ever name the host.
func NewWebhooks(logger *slog.Logger, rawURLs []string, adminURL string) (*Webhooks, error) {
	targets := make([]*url.URL, 0, len(rawURLs))
	for i, raw := range rawURLs {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return nil, fmt.Errorf("review webhook URL #%d is not an absolute http(s) URL", i+1)
		}
		targets = append(targets, u)
	}
	return &Webhooks{
		logger:   logger,
		targets:  targets,
		adminURL: strings.TrimRight(adminURL, "/"),
		client:   &http.Client{Timeout: requestTimeout},
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

func (w *Webhooks) message(req ReviewRequest) string {
	name := req.PluginDisplayName
	if name == "" {
		name = req.PluginName
	}
	text := fmt.Sprintf(":mag: New review requested: **%s** (%#q) version %#q", name, req.PluginName, req.Version)
	if w.adminURL != "" {
		text += fmt.Sprintf("\n[Open submission](%s/admin/submissions/%s)", w.adminURL, req.SubmissionID)
	}
	return text
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
