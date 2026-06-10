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
// the kube-api-proxy. It forwards the caller's Fundament JWT so the proxy can
// authorise the request and inject the per-user ServiceAccount token.
//
// This backend is narrower than Loki: it needs a specific pod, cannot search
// across pods, and only sees logs the node still retains.
type KubeClient struct {
	proxyURL   string // base kube-api-proxy URL (e.g. https://kube-proxy.example)
	authToken  string // caller's bearer token (raw, without "Bearer " prefix)
	httpClient *http.Client
}

// NewKubeClient returns a KubeClient. authToken is the caller's bearer token.
func NewKubeClient(proxyURL, authToken string) *KubeClient {
	return &KubeClient{
		proxyURL:   strings.TrimRight(proxyURL, "/"),
		authToken:  authToken,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (*KubeClient) Backend() Backend { return BackendKubernetes }

func (c *KubeClient) Query(ctx context.Context, p QueryParams) ([]Entry, error) {
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
	defer resp.Body.Close()

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

func (c *KubeClient) Tail(ctx context.Context, p QueryParams) (<-chan Entry, error) {
	if p.Namespace == "" || p.Pod == "" {
		return nil, ErrPodRequired
	}
	resp, err := c.openLogStream(ctx, p, true, 100)
	if err != nil {
		return nil, err
	}

	out := make(chan Entry)
	go func() {
		defer close(out)
		defer resp.Body.Close()
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
func (*KubeClient) Labels(_ context.Context, _, _ string) (Labels, error) {
	return Labels{}, nil
}

func (c *KubeClient) openLogStream(ctx context.Context, p QueryParams, follow bool, tailLines int) (*http.Response, error) {
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()
		return nil, fmt.Errorf("pod logs: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp, nil
}

// lineToEntry parses a single "RFC3339Nano <message>" pod-log line (timestamps=true).
func (c *KubeClient) lineToEntry(line string, p QueryParams) Entry {
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
