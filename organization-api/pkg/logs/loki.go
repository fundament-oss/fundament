package logs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// maxResponseBytes bounds a single backend response. The request side was
// already capped (see MaxLimit) on the grounds that backends preallocate on the
// limit — but nothing bounded what came back, and the content is
// tenant-controlled: 5000 entries of megabyte JSON lines is a multi-gigabyte
// decode.
const maxResponseBytes = 64 << 20

// LokiClient queries a Gardener Vali (credativ's Loki fork) instance over its
// Loki-shaped HTTP API. https://grafana.com/docs/loki/latest/reference/loki-http-api/
//
// Stream label names follow Gardener's logging-stack convention (see the
// label* constants below). Each client targets a single instance; when sourced
// per-shoot from Gardener that instance holds only one cluster's logs, so the
// fundament cluster UUID is not used as a label matcher.
//
// baseURL may include a path prefix (e.g. a Plutono datasource-proxy route); the
// API paths are appended to it, so no separate prefix field is needed.
type LokiClient struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
	// pollInterval paces Tail's query_range polling; a seam for tests, which
	// would otherwise wait whole seconds per poll.
	pollInterval time.Duration
}

// Option configures a LokiClient.
type Option func(*LokiClient)

// WithTransport replaces the HTTP transport, e.g. to trust a private CA for
// the seed ingress (the same bundle the per-shoot Prometheus client uses).
func WithTransport(rt http.RoundTripper) Option {
	return func(c *LokiClient) {
		c.httpClient.Transport = rt
	}
}

// WithPollInterval overrides how often Tail polls query_range for new entries.
// Mainly a test seam; the default paces a live tail at 2s.
func WithPollInterval(d time.Duration) Option {
	return func(c *LokiClient) {
		c.pollInterval = d
	}
}

// NewLokiClient returns a LokiClient targeting the given base URL with no
// authentication (used for the LOGS_URL dev override).
func NewLokiClient(baseURL string, opts ...Option) *LokiClient {
	return NewLokiClientWithAuth(baseURL, "", "", opts...)
}

// NewLokiClientWithAuth returns a LokiClient that sends HTTP basic-auth on every
// request. Empty credentials disable the auth header. Used for the per-shoot
// Vali endpoint, whose credentials come from the Gardener monitoring secret.
func NewLokiClientWithAuth(baseURL, username, password string, opts ...Option) *LokiClient {
	c := &LokiClient{
		baseURL:      strings.TrimRight(baseURL, "/"),
		username:     username,
		password:     password,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		pollInterval: 2 * time.Second,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// newRequest builds a GET request, applying basic-auth when credentials are set.
func (c *LokiClient) newRequest(ctx context.Context, rawURL string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if c.username != "" || c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	return req, nil
}

// do issues a GET, retrying once when the connection was torn down underneath
// us rather than refused semantically — a stale keep-alive or a restarting
// Plutono. The per-shoot Prometheus client already does this against the very
// same seed ingress; the logs client reaching it without a retry made the two
// behave differently on identical infrastructure.
func (c *LokiClient) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req) //nolint:gosec // baseURL comes from the Gardener monitoring secret, not the caller
	if err != nil && isTransientNetErr(err) && ctx.Err() == nil {
		//nolint:bodyclose,gosec // closed by the caller; see above on the URL
		resp, err = c.httpClient.Do(req.Clone(ctx))
	}
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	return resp, nil
}

// isTransientNetErr reports whether err looks like a connection torn down
// underneath us rather than a semantic failure.
func isTransientNetErr(err error) bool {
	return errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF)
}

// apiPrefix is the query-API path prefix. Vali renames Loki's "/loki/api/v1"
// to "/vali/api/v1" (verified against a live Gardener v1.138 shoot,
// 2026-08-04); the legacy "/api/prom" prefix also answers but is deprecated.
const apiPrefix = "/vali/api/v1"

func (*LokiClient) Backend() Backend { return BackendLoki }

func (c *LokiClient) Query(ctx context.Context, p *QueryParams) ([]Entry, error) {
	limit := EffectiveLimit(p.Limit)
	end := p.End
	if end.IsZero() {
		end = time.Now()
	}
	start := p.Start
	if start.IsZero() {
		start = end.Add(-time.Hour)
	}

	u, err := url.Parse(c.baseURL + apiPrefix + "/query_range")
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	q := u.Query()
	q.Set("query", buildLogQL(p))
	q.Set("start", strconv.FormatInt(start.UnixNano(), 10))
	q.Set("end", strconv.FormatInt(end.UnixNano(), 10))
	q.Set("limit", strconv.Itoa(limit))
	q.Set("direction", "backward")
	u.RawQuery = q.Encode()

	streams, err := c.fetchStreams(ctx, u.String())
	if err != nil {
		return nil, err
	}

	entries, err := streamsToEntries(streams, p.ClusterID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})
	// Vali applies the limit per stream, so a multi-stream response can exceed
	// it. Bounding what we asked for is not the same as bounding what we hold.
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

// Tail implements a dependency-free live tail by polling query_range for new
// entries. Loki's native /tail endpoint is a websocket; polling keeps the
// client free of a websocket dependency at the cost of ~poll-interval latency.
func (c *LokiClient) Tail(ctx context.Context, p *QueryParams) (<-chan TailEvent, error) {
	pollInterval := c.pollInterval
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	// A single failed poll is usually a blip (a restarting Plutono, a dropped
	// connection); a run of them is not. Tolerating a few keeps transient
	// noise from tearing down the stream, while a persistent failure still
	// terminates it instead of leaving a silent, permanently empty tail.
	const maxConsecutiveFailures = 3
	out := make(chan TailEvent)
	go func() {
		defer close(out)
		last := time.Now()
		failures := 0
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		fail := func(err error) {
			select {
			case out <- TailEvent{Err: err}:
			case <-ctx.Done():
			}
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				qp := *p
				qp.Start = last
				qp.End = time.Now()
				qp.Limit = 500
				entries, err := c.Query(ctx, &qp)
				if err != nil {
					failures++
					// 401 means the monitoring credentials rotated under us.
					// Polling will never recover from that, so surface it at
					// once and let the caller re-resolve.
					var statusErr *StatusError
					unauthorized := errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusUnauthorized
					if unauthorized || failures >= maxConsecutiveFailures {
						fail(fmt.Errorf("tail poll: %w", err))
						return
					}
					continue
				}
				failures = 0
				// Emit oldest-first so the UI appends in chronological order.
				for i := len(entries) - 1; i >= 0; i-- {
					e := entries[i]
					if !e.Timestamp.After(last) {
						continue
					}
					select {
					case out <- TailEvent{Entry: e}:
					case <-ctx.Done():
						return
					}
				}
				if len(entries) > 0 {
					if t := entries[0].Timestamp; t.After(last) {
						last = t
					}
				}
			}
		}
	}()
	return out, nil
}

func (c *LokiClient) Labels(ctx context.Context, _ /*clusterID*/, namespace string, start, end time.Time) (Labels, error) {
	scope := ""
	if namespace != "" {
		scope = fmt.Sprintf("{%s=%q}", labelNamespace, namespace)
	}
	var (
		labels Labels
		err    error
	)
	if labels.Namespaces, err = c.labelValues(ctx, labelNamespace, "", start, end); err != nil {
		return Labels{}, err
	}
	if labels.Pods, err = c.labelValues(ctx, labelPod, scope, start, end); err != nil {
		return Labels{}, err
	}
	if labels.Containers, err = c.labelValues(ctx, labelContainer, scope, start, end); err != nil {
		return Labels{}, err
	}
	return labels, nil
}

func (c *LokiClient) labelValues(ctx context.Context, name, query string, start, end time.Time) ([]string, error) {
	u, err := url.Parse(c.baseURL + apiPrefix + "/label/" + name + "/values")
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	q := u.Query()
	if query != "" {
		q.Set("query", query)
	}
	if !start.IsZero() {
		q.Set("start", strconv.FormatInt(start.UnixNano(), 10))
	}
	if !end.IsZero() {
		q.Set("end", strconv.FormatInt(end.UnixNano(), 10))
	}
	u.RawQuery = q.Encode()
	req, err := c.newRequest(ctx, u.String())
	if err != nil {
		return nil, err
	}
	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &StatusError{StatusCode: resp.StatusCode, Operation: "loki label values"}
	}
	var result struct {
		Status string   `json:"status"`
		Data   []string `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode label values: %w", err)
	}
	return result.Data, nil
}

func (c *LokiClient) fetchStreams(ctx context.Context, rawURL string) ([]lokiStream, error) {
	req, err := c.newRequest(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &StatusError{StatusCode: resp.StatusCode, Operation: "loki query"}
	}
	var result lokiQueryResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if result.Status != "success" {
		return nil, fmt.Errorf("loki error: %s", result.Status)
	}
	return result.Data.Result, nil
}

// Gardener's logging stack (Fluent-bit + Valitail shipping into Vali) labels
// streams with the Kubernetes-metadata convention, which differs from the bare
// Loki/Promtail defaults: "namespace_name", "pod_name", "container_name"
// (verified against a live Gardener v1.138 shoot, 2026-08-04; streams also
// carry "nodename" and "origin"). They are isolated here so a correction for
// another Gardener version is a one-line change.
const (
	labelNamespace = "namespace_name"
	labelPod       = "pod_name"
	labelContainer = "container_name"
)

// levelLineTokens maps a normalised level onto substrings that a line carrying
// that level plausibly contains. It mirrors NormalizeLevel's prefixes, and is
// deliberately generous: the pre-filter must never be narrower than the exact
// classification, or a real match would be dropped before it is classified.
//
// Level is only ever read from a JSON field on the line or from a stream label,
// never inferred from free text, so a line whose level is ERROR contains the
// token in its own JSON — which is what makes this safe against Vali 2.2.1,
// where there is no structured-metadata filter to use instead.
var levelLineTokens = map[string][]string{
	"ERROR": {"err", "fatal", "crit", "panic", "emerg", "alert"},
	"WARN":  {"warn"},
	"INFO":  {"info", "notice"},
	"DEBUG": {"debug", "trace"},
}

// levelPreFilter returns a case-insensitive LogQL line-filter pattern keeping
// every line that could carry one of the requested levels, or "" when there is
// nothing useful to narrow.
//
// Vali applies the entry limit after the pipeline, so filtering severity in the
// client meant the limit selected the newest lines of *any* level: on a
// namespace logging mostly INFO, a filter for ERROR reported nothing while the
// errors sat just outside the page. Narrowing here makes the limit apply to a
// relevant set. FilterByLevels still enforces the exact filter afterwards, so
// this can only improve recall, never precision.
func levelPreFilter(levels []string) string {
	want := NormalizedLevels(levels)
	if want == nil || len(want) == len(levelLineTokens) {
		// No filter, or every level requested: nothing to narrow.
		return ""
	}
	if want[defaultLevel] {
		// An entry whose level cannot be classified is reported as defaultLevel,
		// and such a line carries no level token at all — so narrowing on a set
		// that includes it would drop exactly those lines. Give up the narrowing
		// rather than lose entries; the case that matters (finding ERRORs on a
		// namespace that is mostly unclassified chatter) does not include it.
		return ""
	}
	tokens := make([]string, 0, len(levelLineTokens))
	for level := range want {
		tokens = append(tokens, levelLineTokens[level]...)
	}
	slices.Sort(tokens)
	return "(?i)" + strings.Join(slices.Compact(tokens), "|")
}

// buildLogQL builds a LogQL query from the params. It always emits at least one
// stream matcher so the query is valid.
func buildLogQL(p *QueryParams) string {
	var matchers []string
	if p.Namespace != "" {
		matchers = append(matchers, fmt.Sprintf("%s=%q", labelNamespace, p.Namespace))
	} else {
		// Ensure a non-empty selector scoped to Kubernetes streams.
		matchers = append(matchers, labelNamespace+`=~".+"`)
	}
	if p.Pod != "" {
		matchers = append(matchers, fmt.Sprintf("%s=%q", labelPod, p.Pod))
	}
	if p.Container != "" {
		matchers = append(matchers, fmt.Sprintf("%s=%q", labelContainer, p.Container))
	}
	query := "{" + strings.Join(matchers, ", ") + "}"
	if p.Search != "" {
		// Case-insensitive, to match MatchesSearch and the console's filter box.
		// A case-sensitive |= meant "Timeout" found nothing on a service logging
		// "timeout", with no hint in the UI that case mattered.
		query += fmt.Sprintf(" |~ %q", "(?i)"+regexp.QuoteMeta(p.Search))
	}
	if pattern := levelPreFilter(p.Levels); pattern != "" {
		query += fmt.Sprintf(" |~ %q", pattern)
	}
	return query
}

func streamsToEntries(streams []lokiStream, clusterID string) ([]Entry, error) {
	var entries []Entry
	for _, s := range streams {
		namespace := s.Stream[labelNamespace]
		pod := s.Stream[labelPod]
		container := s.Stream[labelContainer]
		streamLevel := firstNonEmpty(s.Stream["severity"], s.Stream["level"], s.Stream["detected_level"])
		for _, v := range s.Values {
			if len(v) < 2 {
				continue
			}
			// Never swallow this. A discarded parse error yielded
			// time.Unix(0, 0), and the tail drops everything at-or-before its
			// watermark — so a change in Vali's value format would have produced
			// a permanently empty, permanently error-free stream.
			tsNano, err := strconv.ParseInt(v[0], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parse entry timestamp %q: %w", v[0], err)
			}
			msg, lineLevel, fields := parseLogLine(v[1])
			level := NormalizeLevel(lineLevel)
			if level == "" {
				level = NormalizeLevel(streamLevel)
			}
			if level == "" {
				level = defaultLevel
			}
			entries = append(entries, Entry{
				Timestamp: time.Unix(0, tsNano),
				Level:     level,
				Cluster:   clusterID,
				Namespace: namespace,
				Pod:       pod,
				Container: container,
				Message:   msg,
				Fields:    fields,
			})
		}
	}
	return entries, nil
}

// Loki HTTP API response envelope for query_range (streams result type).

type lokiQueryResponse struct {
	Status string       `json:"status"`
	Data   lokiDataBody `json:"data"`
}

type lokiDataBody struct {
	ResultType string       `json:"resultType"`
	Result     []lokiStream `json:"result"`
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	// Each value is [unixNanoString, logLine].
	Values [][2]string `json:"values"`
}
