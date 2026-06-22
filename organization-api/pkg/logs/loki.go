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

// LokiClient queries a Grafana Loki (or Loki-compatible Vali) instance over its
// HTTP API. https://grafana.com/docs/loki/latest/reference/loki-http-api/
//
// Stream label names follow Gardener's logging-stack convention (see the
// label* constants below). Each client targets a single instance; when sourced
// per-shoot from Gardener that instance holds only one cluster's logs, so the
// fundament cluster UUID is not used as a label matcher.
//
// baseURL may include a path prefix (e.g. a Plutono datasource-proxy route); the
// Loki API paths are appended to it, so no separate prefix field is needed.
type LokiClient struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

// NewLokiClient returns a LokiClient targeting the given base URL with no
// authentication (used for the LOKI_URL dev override).
func NewLokiClient(baseURL string) *LokiClient {
	return NewLokiClientWithAuth(baseURL, "", "")
}

// NewLokiClientWithAuth returns a LokiClient that sends HTTP basic-auth on every
// request. Empty credentials disable the auth header. Used for the per-shoot
// Vali endpoint, whose credentials come from the Gardener monitoring secret.
func NewLokiClientWithAuth(baseURL, username, password string) *LokiClient {
	return &LokiClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		username:   username,
		password:   password,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// newRequest builds a GET request, applying basic-auth when credentials are set.
func (c *LokiClient) newRequest(ctx context.Context, rawURL string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if c.username != "" || c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	return req, nil
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
		scope = fmt.Sprintf("{%s=%q}", labelNamespace, namespace)
	}
	var (
		labels Labels
		err    error
	)
	if labels.Namespaces, err = c.labelValues(ctx, labelNamespace, ""); err != nil {
		return Labels{}, err
	}
	if labels.Pods, err = c.labelValues(ctx, labelPod, scope); err != nil {
		return Labels{}, err
	}
	if labels.Containers, err = c.labelValues(ctx, labelContainer, scope); err != nil {
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
	req, err := c.newRequest(ctx, u.String())
	if err != nil {
		return nil, err
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
	req, err := c.newRequest(ctx, rawURL)
	if err != nil {
		return nil, err
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

// Gardener's logging stack (Fluent-bit + Valitail shipping into Vali) labels
// streams with the Kubernetes-metadata convention, which differs from the bare
// Loki/Promtail defaults: "namespace_name", "pod_name", "container_name".
//
// NOTE: the exact names depend on the Gardener Fluent-bit/Valitail config and
// version — verify against a live Vali (plan Step 0). They are isolated here so
// a correction is a one-line change.
const (
	labelNamespace = "namespace_name"
	labelPod       = "pod_name"
	labelContainer = "container_name"
)

// buildLogQL builds a LogQL query from the params. It always emits at least one
// stream matcher so the query is valid; level filtering is left to the caller
// (the frontend filters by level over the returned set).
func buildLogQL(p QueryParams) string {
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
		query += fmt.Sprintf(" |= %q", p.Search)
	}
	return query
}

func streamsToEntries(streams []lokiStream, clusterID string) []Entry {
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
