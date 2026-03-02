package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client is the interface for querying a Prometheus instance.
type Client interface {
	// Query executes an instant PromQL query at time t and returns the results.
	Query(ctx context.Context, query string, t time.Time) ([]Sample, error)
	// QueryRange executes a PromQL range query and returns time-series results.
	QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]TimeSeries, error)
}

// Sample is a single label-set and scalar value from an instant query result.
type Sample struct {
	Labels map[string]string
	Value  float64
}

// TimeSeries is a sequence of data points for a single label-set.
type TimeSeries struct {
	Labels  map[string]string
	Samples []DataPoint
}

// DataPoint is a single (time, value) pair within a time-series.
type DataPoint struct {
	Time  time.Time
	Value float64
}

// HTTPClient queries a real Prometheus instance over HTTP using the standard
// Prometheus HTTP API (/api/v1/query and /api/v1/query_range).
type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewHTTPClient returns an HTTPClient targeting the given Prometheus base URL.
func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *HTTPClient) Query(ctx context.Context, query string, t time.Time) ([]Sample, error) {
	u, err := url.Parse(c.baseURL + "/api/v1/query")
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	q := u.Query()
	q.Set("query", query)
	q.Set("time", strconv.FormatInt(t.Unix(), 10))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if result.Status != "success" {
		return nil, fmt.Errorf("prometheus error: %s", result.Error)
	}

	return result.Data.toSamples()
}

func (c *HTTPClient) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]TimeSeries, error) {
	u, err := url.Parse(c.baseURL + "/api/v1/query_range")
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	q := u.Query()
	q.Set("query", query)
	q.Set("start", strconv.FormatInt(start.Unix(), 10))
	q.Set("end", strconv.FormatInt(end.Unix(), 10))
	q.Set("step", strconv.FormatInt(int64(step.Seconds()), 10))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if result.Status != "success" {
		return nil, fmt.Errorf("prometheus error: %s", result.Error)
	}

	return result.Data.toTimeSeries()
}

// Internal types for decoding the Prometheus HTTP API JSON response envelope.
// https://prometheus.io/docs/prometheus/latest/querying/api/

type apiResponse struct {
	Status string      `json:"status"`
	Error  string      `json:"error"`
	Data   apiDataBody `json:"data"`
}

type apiDataBody struct {
	ResultType string            `json:"resultType"`
	Result     []json.RawMessage `json:"result"`
}

func (d apiDataBody) toSamples() ([]Sample, error) {
	// Vector result: each item is {"metric":{...},"value":[unixTs,"valStr"]}
	var out []Sample
	for _, raw := range d.Result {
		var item struct {
			Metric map[string]string  `json:"metric"`
			Value  [2]json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, fmt.Errorf("parse sample: %w", err)
		}
		var valStr string
		if err := json.Unmarshal(item.Value[1], &valStr); err != nil {
			return nil, fmt.Errorf("parse value string: %w", err)
		}
		f, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			return nil, fmt.Errorf("parse float: %w", err)
		}
		out = append(out, Sample{Labels: item.Metric, Value: f})
	}
	return out, nil
}

func (d apiDataBody) toTimeSeries() ([]TimeSeries, error) {
	// Matrix result: each item is {"metric":{...},"values":[[unixTs,"valStr"],...]}
	var out []TimeSeries
	for _, raw := range d.Result {
		var item struct {
			Metric map[string]string    `json:"metric"`
			Values [][2]json.RawMessage `json:"values"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, fmt.Errorf("parse timeseries: %w", err)
		}
		var points []DataPoint
		for _, v := range item.Values {
			var tsFloat float64
			if err := json.Unmarshal(v[0], &tsFloat); err != nil {
				return nil, fmt.Errorf("parse timestamp: %w", err)
			}
			var valStr string
			if err := json.Unmarshal(v[1], &valStr); err != nil {
				return nil, fmt.Errorf("parse value string: %w", err)
			}
			f, err := strconv.ParseFloat(valStr, 64)
			if err != nil {
				return nil, fmt.Errorf("parse float: %w", err)
			}
			points = append(points, DataPoint{
				Time:  time.Unix(int64(tsFloat), 0),
				Value: f,
			})
		}
		out = append(out, TimeSeries{Labels: item.Metric, Samples: points})
	}
	return out, nil
}
