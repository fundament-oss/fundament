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

// maxFanOutStreams caps how many pod and container logs a query or tail
// without a pod reads in total. A plugin namespace holds a handful of pods;
// the cap keeps a large namespace from turning one query into hundreds of
// kubelet requests. Past it, the pods the listing returns last are left out
// without notice: the API has no way to report a partial read yet.
const maxFanOutStreams = 20

// defaultRelistInterval is how often a tail across a namespace lists its pods
// again, to follow the ones it does not follow yet: pods started (or done
// starting) since, and pods whose stream ended or failed.
const defaultRelistInterval = 10 * time.Second

// KubeClient reads container logs from the Kubernetes pod-log endpoint through
// the kube-api-proxy. It forwards the caller's own credentials (bearer token
// or session cookie) so the proxy can authorise the request and inject the
// per-user ServiceAccount token.
//
// This backend is narrower than Loki: it needs a namespace, and when no pod is
// given reads the namespace's pods one request per container, up to
// maxFanOutStreams in all. It only sees logs the node still retains.
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
	// relistInterval is defaultRelistInterval; tests shorten it.
	relistInterval time.Duration
}

// NewKubeClient returns a KubeClient. auth carries the caller's credential
// headers — Authorization for token clients, Cookie for the browser session —
// and is forwarded on every request.
func NewKubeClient(proxyURL string, auth http.Header) *KubeClient {
	return &KubeClient{
		proxyURL:       strings.TrimRight(proxyURL, "/"),
		auth:           auth,
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		followClient:   &http.Client{},
		relistInterval: defaultRelistInterval,
	}
}

func (*KubeClient) Backend() Backend { return BackendKubernetes }

func (c *KubeClient) Query(ctx context.Context, p *QueryParams) ([]Entry, error) {
	if p.Namespace == "" {
		return nil, ErrNamespaceRequired
	}
	if p.Pod != "" {
		return c.queryPod(ctx, p, MaxLimit)
	}

	targets, err := c.listTargets(ctx, p, false)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, nil
	}
	// A filter makes each pod's read fetch more lines than the caller asked
	// for (see queryPod). Across pods that budget is shared, not spent per
	// pod: 20 pods at MaxLimit each is 100k lines parsed for one page.
	fetchBudget := max(EffectiveLimit(p.Limit), MaxLimit/len(targets))
	results := make([][]Entry, len(targets))
	errs := make([]error, len(targets))
	var wg sync.WaitGroup
	for i := range targets {
		wg.Go(func() {
			results[i], errs[i] = c.queryPod(ctx, &targets[i], fetchBudget)
		})
	}
	wg.Wait()

	// One pod that cannot be read (a container still starting, a node that
	// went away) should not hide the logs of the others; only when none could
	// be read is there nothing to show but the error. Like the cap in
	// listTargets, a skipped pod goes unreported: the API has no field for it.
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

// queryPod reads one pod's log, newest first. filteredFetch is how many lines
// it reads when a filter is active: MaxLimit for one pod, a share of it when
// a namespace's pods are read together.
func (c *KubeClient) queryPod(ctx context.Context, p *QueryParams, filteredFetch int) ([]Entry, error) {
	limit := EffectiveLimit(p.Limit)

	// The pod-log endpoint has no server-side search and no end bound — it can
	// only hand back the last N raw lines — so Search, End and Levels have to be
	// applied here. That means fetching more than the caller asked for when a
	// filter is active: applying the kubelet's cap to *unfiltered* lines meant a
	// match just outside the last `limit` lines was reported as "no results",
	// and an explicit End was ignored outright.
	fetchLines := limit
	if p.Search != "" || !p.End.IsZero() || len(p.Levels) > 0 {
		fetchLines = max(limit, filteredFetch)
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
	return c.tailNamespace(ctx, p)
}

// tailNamespace follows every running pod in the namespace as one tail. The
// pods change while it runs (a rollout, a scale-up, a pod done starting), so
// it lists them again every relistInterval and follows the new ones. A pod
// whose stream ends or fails is dropped, and followed again on the next
// listing if it still runs: one pod going away does not end the tail. Only a
// failed listing does, as that is the namespace becoming unreadable (access
// revoked, cluster gone) rather than one pod.
func (c *KubeClient) tailNamespace(parent context.Context, p *QueryParams) (<-chan TailEvent, error) {
	targets, err := c.listTargets(parent, p, true)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(parent)
	t := &namespaceTail{client: c, ctx: ctx, out: make(chan TailEvent), following: map[string]bool{}}
	// Pods that run but none of which can be followed (no access to pod
	// logs, say) are an error to report now, not a tail that stays quiet.
	if opened, err := t.follow(targets); opened == 0 && err != nil {
		cancel()
		return nil, err
	}

	go func() {
		defer cancel()
		err := t.watch(p)
		// Every pod's forwarder stops before the channel closes, so the error,
		// if any, is the last event, as TailEvent promises.
		cancel()
		t.forwarders.Wait()
		if err != nil {
			select {
			case t.out <- TailEvent{Err: err}:
			case <-parent.Done():
			}
		}
		close(t.out)
	}()
	return t.out, nil
}

// namespaceTail is one tail across a namespace's pods.
type namespaceTail struct {
	client *KubeClient
	ctx    context.Context //nolint:containedctx // the tail's own lifetime, shared by its goroutines
	out    chan TailEvent
	// forwarders counts the per-pod goroutines writing to out.
	forwarders sync.WaitGroup
	mu         sync.Mutex
	// following holds the pod/container pairs with an open stream.
	following map[string]bool
}

// watch lists the pods every relistInterval and follows the ones not
// followed yet, until the tail is cancelled (nil) or a listing fails.
func (t *namespaceTail) watch(p *QueryParams) error {
	ticker := time.NewTicker(t.client.relistInterval)
	defer ticker.Stop()
	for {
		select {
		case <-t.ctx.Done():
			return nil
		case <-ticker.C:
		}
		targets, err := t.client.listTargets(t.ctx, p, true)
		if err != nil {
			if t.ctx.Err() != nil {
				return nil
			}
			return err
		}
		_, _ = t.follow(targets) // a pod that cannot be followed now is tried again next time
	}
}

// follow opens a stream for every target not followed yet, all at once: one
// after another, a slow kubelet holds up the rest, and with tailLines=0 every
// line written meanwhile is lost. It returns how many it opened and the first
// error.
func (t *namespaceTail) follow(targets []QueryParams) (int, error) {
	t.mu.Lock()
	var todo []QueryParams
	for i := range targets {
		if key := tailKey(&targets[i]); !t.following[key] {
			t.following[key] = true
			todo = append(todo, targets[i])
		}
	}
	t.mu.Unlock()

	streams := make([]<-chan TailEvent, len(todo))
	errs := make([]error, len(todo))
	var opening sync.WaitGroup
	for i := range todo {
		opening.Go(func() {
			streams[i], errs[i] = t.client.tailPod(t.ctx, &todo[i])
		})
	}
	opening.Wait()

	opened := 0
	var firstErr error
	for i := range todo {
		key := tailKey(&todo[i])
		if errs[i] != nil {
			t.release(key)
			if firstErr == nil {
				firstErr = errs[i]
			}
			continue
		}
		opened++
		t.forwarders.Go(func() {
			defer t.release(key)
			t.forward(streams[i])
		})
	}
	return opened, firstErr
}

// forward passes one pod's entries on. An error ends only that pod's stream;
// the next listing follows the pod again if it still runs.
func (t *namespaceTail) forward(stream <-chan TailEvent) {
	for ev := range stream {
		if ev.Err != nil {
			return
		}
		select {
		case t.out <- ev:
		case <-t.ctx.Done():
			return
		}
	}
}

func (t *namespaceTail) release(key string) {
	t.mu.Lock()
	delete(t.following, key)
	t.mu.Unlock()
}

func tailKey(p *QueryParams) string { return p.Pod + "/" + p.Container }

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
// Pending pods are skipped, as they have not written a line yet. A tail
// (runningOnly) also skips the pods that are done: following one returns
// straight away, only to be opened again on the next listing.
func (c *KubeClient) listTargets(ctx context.Context, p *QueryParams, runningOnly bool) ([]QueryParams, error) {
	endpoint := fmt.Sprintf("%s/clusters/%s/api/v1/namespaces/%s/pods",
		c.proxyURL,
		url.PathEscape(p.ClusterID),
		url.PathEscape(p.Namespace))
	resp, err := c.get(ctx, c.httpClient, endpoint, "list pods")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var pods podList
	if err := json.NewDecoder(resp.Body).Decode(&pods); err != nil {
		return nil, fmt.Errorf("decode pod list: %w", err)
	}

	var targets []QueryParams
	for _, pod := range pods.Items {
		if pod.Status.Phase == "Pending" || (runningOnly && pod.Status.Phase != "Running") {
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

	client := c.httpClient
	if follow {
		client = c.followClient
	}
	return c.get(ctx, client, u.String(), "pod logs")
}

// get sends a GET through the kube-api-proxy with the caller's credentials.
func (c *KubeClient) get(ctx context.Context, client *http.Client, endpoint, operation string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	maps.Copy(req.Header, c.auth)

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
		return nil, &StatusError{StatusCode: resp.StatusCode, Operation: operation}
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
