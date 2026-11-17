package logs

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrNamespaceRequired is returned by the Kubernetes fallback when a query
// does not name a namespace. Unlike Loki, the pod-log endpoint reads one pod
// at a time, so "every pod" has to be a namespace's pods, listed first.
var ErrNamespaceRequired = errors.New("kubernetes log backend requires a namespace")

// maxFanOutStreams caps how many pod logs one query without a pod reads at
// once. A plugin namespace holds a handful of pods; the cap keeps a large
// namespace from turning one query into hundreds of kubelet requests.
const maxFanOutStreams = 20

// KubeClient reads container logs from the Kubernetes pod-log endpoint through
// the kube-api-proxy. It forwards the caller's own credentials (bearer token
// or session cookie) so the proxy can authorise the request and inject the
// per-user ServiceAccount token.
//
// This backend is narrower than Loki: it needs a namespace, reads every pod in
// it one request each when no pod is given, and only sees logs the node still
// retains.
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
	if p.Namespace == "" {
		return nil, ErrNamespaceRequired
	}
	if p.Pod != "" {
		return c.queryPod(ctx, p)
	}

	targets, err := c.listTargets(ctx, p)
	if err != nil {
		return nil, err
	}
	results := make([][]Entry, len(targets))
	errs := make([]error, len(targets))
	var wg sync.WaitGroup
	for i := range targets {
		wg.Go(func() {
			results[i], errs[i] = c.queryPod(ctx, &targets[i])
		})
	}
	wg.Wait()

	// One pod that cannot be read (a container still starting, a node that
	// went away) should not hide the logs of the others; only when none could
	// be read is there nothing to show but the error.
	var merged []Entry
	var firstErr error
	read := 0
	for i := range targets {
		if errs[i] != nil {
			if firstErr == nil {
				firstErr = errs[i]
			}
			continue
		}
		read++
		merged = append(merged, results[i]...)
	}
	if read == 0 && firstErr != nil {
		return nil, firstErr
	}

	slices.SortStableFunc(merged, func(a, b Entry) int { return b.Timestamp.Compare(a.Timestamp) })
	if limit := EffectiveLimit(p.Limit); len(merged) > limit {
		merged = merged[:limit]
	}
	return merged, nil
}

// queryPod reads one pod's log, newest first.
func (c *KubeClient) queryPod(ctx context.Context, p *QueryParams) ([]Entry, error) {
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
	if p.Namespace == "" {
		return nil, ErrNamespaceRequired
	}
	if p.Pod != "" {
		return c.tailPod(ctx, p)
	}

	targets, err := c.listTargets(ctx, p)
	if err != nil {
		return nil, err
	}
	streams := make([]<-chan TailEvent, 0, len(targets))
	var firstErr error
	for i := range targets {
		stream, err := c.tailPod(ctx, &targets[i])
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		streams = append(streams, stream)
	}
	if len(streams) == 0 {
		if firstErr != nil {
			return nil, firstErr
		}
		// No pods yet: stay open and quiet, as a tail on a quiet pod does.
		out := make(chan TailEvent)
		go func() {
			defer close(out)
			<-ctx.Done()
		}()
		return out, nil
	}
	return mergeTails(ctx, streams), nil
}

// mergeTails fans several pod tails into one. It closes when every tail has
// ended. A read error on one pod ends the merged tail, the same as it ends a
// single pod's: the caller cannot tell from a quiet stream that a pod dropped.
func mergeTails(ctx context.Context, streams []<-chan TailEvent) <-chan TailEvent {
	out := make(chan TailEvent)
	var wg sync.WaitGroup
	for _, stream := range streams {
		wg.Go(func() {
			for ev := range stream {
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				}
			}
		})
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

// tailPod follows one pod's log.
func (c *KubeClient) tailPod(ctx context.Context, p *QueryParams) (<-chan TailEvent, error) {
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

// podList is the part of a Kubernetes PodList that fan-out reads.
type podList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Spec struct {
			Containers []struct {
				Name string `json:"name"`
			} `json:"containers"`
		} `json:"spec"`
		Status struct {
			Phase string `json:"phase"`
		} `json:"status"`
	} `json:"items"`
}

// listTargets turns a query without a pod into one query per pod and
// container in the namespace. Every container, unless the query names one:
// the pod-log endpoint refuses a multi-container pod without a container.
// Pending pods are skipped, as they have not written a line yet.
func (c *KubeClient) listTargets(ctx context.Context, p *QueryParams) ([]QueryParams, error) {
	endpoint := fmt.Sprintf("%s/clusters/%s/api/v1/namespaces/%s/pods",
		c.proxyURL,
		url.PathEscape(p.ClusterID),
		url.PathEscape(p.Namespace))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	maps.Copy(req.Header, c.auth)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 2048))
		return nil, &StatusError{StatusCode: resp.StatusCode, Operation: "list pods"}
	}

	var pods podList
	if err := json.NewDecoder(resp.Body).Decode(&pods); err != nil {
		return nil, fmt.Errorf("decode pod list: %w", err)
	}

	var targets []QueryParams
	for _, pod := range pods.Items {
		if pod.Status.Phase == "Pending" {
			continue
		}
		for _, container := range pod.Spec.Containers {
			if p.Container != "" && container.Name != p.Container {
				continue
			}
			if len(targets) == maxFanOutStreams {
				return targets, nil
			}
			target := *p
			target.Pod = pod.Metadata.Name
			target.Container = container.Name
			targets = append(targets, target)
		}
	}
	return targets, nil
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
	maps.Copy(req.Header, c.auth)

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

// Histogram counts the entries this backend can fetch, which is a bounded page
// of one pod's log rather than the window: the pod-log endpoint returns lines,
// not aggregates, so there is nothing to ask it for. The result is reported as
// inexact, and callers are expected to say so — the whole point of moving the
// histogram server-side was to stop presenting a page as a total.
func (c *KubeClient) Histogram(ctx context.Context, p *HistogramParams) (Histogram, error) {
	return histogramFromQuery(ctx, c, p)
}
