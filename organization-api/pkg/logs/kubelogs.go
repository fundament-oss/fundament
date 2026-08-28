package logs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrPodRequired is returned by the Kubernetes fallback when a query does not
// identify a specific namespace and pod. Unlike Loki, the pod-log endpoint can
// only read one pod at a time.
var ErrPodRequired = errors.New("kubernetes log backend requires a namespace and pod")

// KubeClient reads container logs from the Kubernetes pod-log endpoint through
// the kube-api-proxy. It forwards the caller's own credentials (bearer token
// or session cookie) so the proxy can authorise the request and inject the
// per-user ServiceAccount token.
//
// This backend is narrower than Loki: it needs a specific pod, cannot search
// across pods, and only sees logs the node still retains.
type KubeClient struct {
	proxyURL string      // base kube-api-proxy URL (e.g. http://kube-api-proxy:8081)
	auth     http.Header // caller's auth headers (Authorization and/or Cookie), forwarded verbatim
	// httpClient bounds one-shot reads. Its Timeout also covers reading the
	// response body, which is why the follow path cannot use it.
	httpClient *http.Client
	// followClient serves ?follow=true streams, which are long-lived by
	// design: a Client.Timeout would cut the body read mid-stream (30s in,
	// every time). Cancellation comes from the request context instead.
	followClient *http.Client
}

// NewKubeClient returns a KubeClient. auth carries the caller's credential
// headers — Authorization for token clients, Cookie for the browser session —
// and is forwarded on every request.
func NewKubeClient(proxyURL string, auth http.Header) *KubeClient {
	return &KubeClient{
		proxyURL:     strings.TrimRight(proxyURL, "/"),
		auth:         auth,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		followClient: &http.Client{},
	}
}

func (*KubeClient) Backend() Backend { return BackendKubernetes }

func (c *KubeClient) Query(ctx context.Context, p *QueryParams) ([]Entry, error) {
	if p.Namespace == "" || p.Pod == "" {
		return nil, ErrPodRequired
	}
	limit := EffectiveLimit(p.Limit)

	// The pod-log endpoint has no server-side search and no end bound — it can
	// only hand back the last N raw lines — so Search, End and Levels have to be
	// applied here. That means fetching more than the caller asked for when a
	// filter is active: applying the kubelet's cap to *unfiltered* lines meant a
	// match just outside the last `limit` lines was reported as "no results",
	// and an explicit End was ignored outright.
	fetchLines := limit
	if p.Search != "" || !p.End.IsZero() || len(p.Levels) > 0 {
		fetchLines = MaxLimit
	}

	resp, err := c.openLogStream(ctx, p, false, fetchLines)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	entries := make([]Entry, 0, min(fetchLines, 1024))
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		entries = append(entries, c.lineToEntry(scanner.Text(), p))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read pod logs: %w", err)
	}

	// The pod-log endpoint returns oldest-first; reverse to newest-first.
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}

	entries = filterEntries(entries, p)
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

// filterEntries applies the query filters the pod-log endpoint cannot express.
// Search matches case-insensitively, the same way the Vali line filter and the
// console's own filter box do.
func filterEntries(entries []Entry, p *QueryParams) []Entry {
	entries = FilterByLevels(entries, p.Levels)
	if p.Search == "" && p.End.IsZero() {
		return entries
	}
	out := make([]Entry, 0, len(entries))
	for i := range entries {
		if !p.End.IsZero() && entries[i].Timestamp.After(p.End) {
			continue
		}
		if !MatchesSearch(entries[i].Message, p.Search) {
			continue
		}
		out = append(out, entries[i])
	}
	return out
}

func (c *KubeClient) Tail(ctx context.Context, p *QueryParams) (<-chan TailEvent, error) {
	if p.Namespace == "" || p.Pod == "" {
		return nil, ErrPodRequired
	}
	// tailLines=0: emit only lines written after the stream opens. Replaying
	// history here would duplicate what the caller already fetched via Query
	// (the Vali tail likewise starts at "now").
	resp, err := c.openLogStream(ctx, p, true, 0) //nolint:bodyclose // closed by the reader goroutine below
	if err != nil {
		return nil, err
	}

	out := make(chan TailEvent)
	go func() {
		defer close(out)
		defer func() { _ = resp.Body.Close() }()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		wantLevels := NormalizedLevels(p.Levels)
		for scanner.Scan() {
			// The pod-log endpoint cannot filter, so the tail applies the same
			// filters Query does; otherwise a filtered tail streams every line.
			entry := c.lineToEntry(scanner.Text(), p)
			if !MatchesSearch(entry.Message, p.Search) {
				continue
			}
			if wantLevels != nil && !wantLevels[NormalizeLevel(entry.Level)] {
				continue
			}
			select {
			case out <- TailEvent{Entry: entry}:
			case <-ctx.Done():
				return
			}
		}
		// Without this, a read failure ends the tail exactly like a clean EOF
		// does — the channel closes and the cause is lost.
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			select {
			case out <- TailEvent{Err: fmt.Errorf("read pod log stream: %w", err)}:
			case <-ctx.Done():
			}
		}
	}()
	return out, nil
}

// Labels is not supported by the pod-log endpoint; the frontend falls back to
// the cluster/namespace listing APIs to populate filters.
func (*KubeClient) Labels(_ context.Context, _, _ string, _, _ time.Time) (Labels, error) {
	return Labels{}, nil
}

func (c *KubeClient) openLogStream(ctx context.Context, p *QueryParams, follow bool, tailLines int) (*http.Response, error) {
	// Escape the caller-supplied segments. Unescaped, a pod named
	// "x?previous=true" terminates the path and injects a query parameter that
	// survives q.Set, and "%2f..%2f.." reaches the proxy with its dot segments
	// un-normalized — org-api would fetch an attacker-chosen kube API path with
	// the caller's credentials attached. It also simply queries the wrong
	// resource for any legitimate name with an unusual character.
	endpoint := fmt.Sprintf("%s/clusters/%s/api/v1/namespaces/%s/pods/%s/log",
		c.proxyURL,
		url.PathEscape(p.ClusterID),
		url.PathEscape(p.Namespace),
		url.PathEscape(p.Pod))
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	q := u.Query()
	q.Set("timestamps", "true")
	q.Set("tailLines", strconv.Itoa(tailLines))
	if follow {
		q.Set("follow", "true")
	}
	if p.Container != "" {
		q.Set("container", p.Container)
	}
	if !p.Start.IsZero() {
		since := int64(time.Since(p.Start).Seconds())
		if since > 0 {
			q.Set("sinceSeconds", strconv.FormatInt(since, 10))
		}
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	for name, values := range c.auth {
		req.Header[name] = values
	}

	client := c.httpClient
	if follow {
		client = c.followClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Drain a bounded amount so the connection can be reused, but keep the
		// upstream body out of the error: it reaches the browser verbatim.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 2048))
		_ = resp.Body.Close()
		// A *StatusError, like the Vali path returns, so one classification
		// covers both backends. A plain error here meant every kube-path failure
		// bypassed degradation and arrived as CodeInternal.
		return nil, &StatusError{StatusCode: resp.StatusCode, Operation: "pod logs"}
	}
	return resp, nil
}

// lineToEntry parses a single "RFC3339Nano <message>" pod-log line (timestamps=true).
func (c *KubeClient) lineToEntry(line string, p *QueryParams) Entry {
	ts := time.Now()
	rest := line
	if idx := strings.IndexByte(line, ' '); idx > 0 {
		if parsed, err := time.Parse(time.RFC3339Nano, line[:idx]); err == nil {
			ts = parsed
			rest = line[idx+1:]
		}
	}
	msg, lineLevel, fields := parseLogLine(rest)
	level := NormalizeLevel(lineLevel)
	if level == "" {
		level = defaultLevel
	}
	return Entry{
		Timestamp: ts,
		Level:     level,
		Cluster:   p.ClusterID,
		Namespace: p.Namespace,
		Pod:       p.Pod,
		Container: p.Container,
		Message:   msg,
		Fields:    fields,
	}
}
