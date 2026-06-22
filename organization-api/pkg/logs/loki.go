package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// LokiClient queries a Grafana Loki instance over its HTTP API.
// https://grafana.com/docs/loki/latest/reference/loki-http-api/
//
// Stream label names are assumed to follow the common Promtail/Alloy Kubernetes
// convention (namespace, pod, container). A single Loki instance is assumed to
// be scoped to the relevant clusters; the fundament cluster UUID is not used as
// a label matcher.
type LokiClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewLokiClient returns a LokiClient targeting the given Loki base URL.
func NewLokiClient(baseURL string) *LokiClient {
	return &LokiClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (*LokiClient) Backend() Backend { return BackendLoki }

func (c *LokiClient) Query(ctx context.Context, p QueryParams) ([]Entry, error) {
	limit := p.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	end := p.End
	if end.IsZero() {
		end = time.Now()
	}
	start := p.Start
	if start.IsZero() {
		start = end.Add(-time.Hour)
	}

	u, err := url.Parse(c.baseURL + "/loki/api/v1/query_range")
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

	entries := streamsToEntries(streams, p.ClusterID)
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})
	return entries, nil
}

// Tail implements a dependency-free live tail by polling query_range for new
// entries. Loki's native /tail endpoint is a websocket; polling keeps the
// client free of a websocket dependency at the cost of ~poll-interval latency.
func (c *LokiClient) Tail(ctx context.Context, p QueryParams) (<-chan Entry, error) {
	const pollInterval = 2 * time.Second
	out := make(chan Entry)
	go func() {
		defer close(out)
		last := time.Now()
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				qp := p
				qp.Start = last
				qp.End = time.Now()
				qp.Limit = 500
				entries, err := c.Query(ctx, qp)
				if err != nil {
					continue
				}
				// Emit oldest-first so the UI appends in chronological order.
				for i := len(entries) - 1; i >= 0; i-- {
					e := entries[i]
					if !e.Timestamp.After(last) {
						continue
					}
					select {
					case out <- e:
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

func (c *LokiClient) Labels(ctx context.Context, _ /*clusterID*/, namespace string) (Labels, error) {
	scope := ""
	if namespace != "" {
		scope = fmt.Sprintf(`{namespace=%q}`, namespace)
	}
	var (
		labels Labels
		err    error
	)
	if labels.Namespaces, err = c.labelValues(ctx, "namespace", ""); err != nil {
		return Labels{}, err
	}
	if labels.Pods, err = c.labelValues(ctx, "pod", scope); err != nil {
		return Labels{}, err
	}
	if labels.Containers, err = c.labelValues(ctx, "container", scope); err != nil {
		return Labels{}, err
	}
	return labels, nil
}

func (c *LokiClient) labelValues(ctx context.Context, name, query string) ([]string, error) {
	u, err := url.Parse(c.baseURL + "/loki/api/v1/label/" + name + "/values")
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	if query != "" {
		q := u.Query()
		q.Set("query", query)
		u.RawQuery = q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("loki label values: status %d", resp.StatusCode)
	}
	var result struct {
		Status string   `json:"status"`
		Data   []string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode label values: %w", err)
	}
	return result.Data, nil
}

func (c *LokiClient) fetchStreams(ctx context.Context, rawURL string) ([]lokiStream, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("loki query: status %d", resp.StatusCode)
	}
	var result lokiQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if result.Status != "success" {
		return nil, fmt.Errorf("loki error: %s", result.Status)
	}
	return result.Data.Result, nil
}

// buildLogQL builds a LogQL query from the params. It always emits at least one
// stream matcher so the query is valid; level filtering is left to the caller
// (the frontend filters by level over the returned set).
func buildLogQL(p QueryParams) string {
	var matchers []string
	if p.Namespace != "" {
		matchers = append(matchers, fmt.Sprintf("namespace=%q", p.Namespace))
	} else {
		// Ensure a non-empty selector scoped to Kubernetes streams.
		matchers = append(matchers, `namespace=~".+"`)
	}
	if p.Pod != "" {
		matchers = append(matchers, fmt.Sprintf("pod=%q", p.Pod))
	}
	if p.Container != "" {
		matchers = append(matchers, fmt.Sprintf("container=%q", p.Container))
	}
	query := "{" + strings.Join(matchers, ", ") + "}"
	if p.Search != "" {
		query += fmt.Sprintf(" |= %q", p.Search)
	}
	return query
}

func streamsToEntries(streams []lokiStream, clusterID string) []Entry {
	var entries []Entry
	for _, s := range streams {
		namespace := s.Stream["namespace"]
		pod := s.Stream["pod"]
		container := s.Stream["container"]
		streamLevel := firstNonEmpty(s.Stream["level"], s.Stream["detected_level"])
		for _, v := range s.Values {
			if len(v) < 2 {
				continue
			}
			tsNano, _ := strconv.ParseInt(v[0], 10, 64)
			msg, lineLevel, fields := parseLogLine(v[1])
			level := normalizeLevel(lineLevel)
			if level == "" {
				level = normalizeLevel(streamLevel)
			}
			if level == "" {
				level = "INFO"
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
	return entries
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
