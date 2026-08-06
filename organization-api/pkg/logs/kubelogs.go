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
	proxyURL   string      // base kube-api-proxy URL (e.g. http://kube-api-proxy:8081)
	auth       http.Header // caller's auth headers (Authorization and/or Cookie), forwarded verbatim
	httpClient *http.Client
}

// NewKubeClient returns a KubeClient. auth carries the caller's credential
// headers — Authorization for token clients, Cookie for the browser session —
// and is forwarded on every request.
func NewKubeClient(proxyURL string, auth http.Header) *KubeClient {
	return &KubeClient{
		proxyURL:   strings.TrimRight(proxyURL, "/"),
		auth:       auth,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (*KubeClient) Backend() Backend { return BackendKubernetes }

func (c *KubeClient) Query(ctx context.Context, p *QueryParams) ([]Entry, error) {
	if p.Namespace == "" || p.Pod == "" {
		return nil, ErrPodRequired
	}
	limit := p.Limit
	if limit <= 0 {
		limit = defaultLimit
	}

	resp, err := c.openLogStream(ctx, p, false, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var entries []Entry
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
	return entries, nil
}

func (c *KubeClient) Tail(ctx context.Context, p *QueryParams) (<-chan Entry, error) {
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

	out := make(chan Entry)
	go func() {
		defer close(out)
		defer func() { _ = resp.Body.Close() }()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			select {
			case out <- c.lineToEntry(scanner.Text(), p):
			case <-ctx.Done():
				return
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
	endpoint := fmt.Sprintf("%s/clusters/%s/api/v1/namespaces/%s/pods/%s/log",
		c.proxyURL, p.ClusterID, p.Namespace, p.Pod)
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("pod logs: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
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
	level := normalizeLevel(lineLevel)
	if level == "" {
		level = "INFO"
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
