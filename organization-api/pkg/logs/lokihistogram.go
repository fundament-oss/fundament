package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// jsonLevelKey matches the severity key of a JSON log line, up to the start of
// its value. The key set is levelKeys, which is what parseLogLine reads.
const jsonLevelKey = `"(level|severity|lvl|loglevel|log_level)" *: *"?`

// levelValuePrefixes are the value prefixes NormalizeLevel classifies onto each
// level. Kept beside jsonLevelKey so the LogQL filter and the Go classifier
// cannot drift apart silently.
var levelValuePrefixes = map[string][]string{
	"ERROR": {"err", "fatal", "crit", "panic", "emerg", "alert"},
	"WARN":  {"warn"},
	"INFO":  {"info", "notice"},
	"DEBUG": {"debug", "trace"},
}

// klogLevelLetter is the leading severity letter of a klog/glog header for each
// level, mirroring klogSeverity. Gardener's system components log in klog and
// per-shoot Vali holds exactly those streams, so this half of the match carries
// most of the traffic.
var klogLevelLetter = map[string]string{
	"ERROR": "[EF]",
	"WARN":  "W",
	"INFO":  "I",
	"DEBUG": "D",
}

// levelLineMatch returns a LogQL line-filter pattern for lines that
// parseLogLine would classify as level.
//
// Unlike levelPreFilter — which is deliberately generous because an exact
// filter runs after it — nothing re-checks this one: a counted line is counted.
// So it matches the two shapes parseLogLine actually reads a severity from, a
// JSON severity key and a klog header, rather than any occurrence of the word
// anywhere in the line. The klog half stays case-sensitive because the header's
// severity letter is uppercase by definition.
func levelLineMatch(level string) string {
	return `(?i:` + jsonLevelKey + `(` + strings.Join(levelValuePrefixes[level], "|") + `))` +
		`|^` + klogLevelLetter[level] + `[0-9]{4} `
}

// Histogram counts entries per severity per bucket with Vali's metric API, so
// the answer covers the whole window instead of the newest Limit lines.
//
// ERROR, WARN and DEBUG are counted directly. INFO is the remainder — total
// minus the other three — because that is what makes the chart agree with the
// entry list: parseLogLine reports an unclassifiable line as defaultLevel
// (INFO), and no line filter can match "carries no severity at all". Deriving
// it also means the stack always adds up to the true total, so the shape of
// the chart is right even where the split is approximate.
func (c *LokiClient) Histogram(ctx context.Context, p *HistogramParams) (Histogram, error) {
	start, end, buckets, step := p.window()
	if step <= 0 {
		return Histogram{Buckets: emptyBuckets(start, buckets, step), Exact: true}, nil
	}
	// Vali takes a range vector in whole seconds, so the bucket width is
	// truncated to a second and the series is anchored to the end of the
	// window: buckets tile it exactly, at the cost of at most one second per
	// bucket of lead-in on a range that does not divide evenly.
	step = step.Truncate(time.Second)
	if step < time.Second {
		step = time.Second
	}
	start = end.Add(-time.Duration(buckets) * step)

	// The level filter is applied per series below, so the shared selector must
	// not carry levelPreFilter's narrowing as well.
	selector := p.QueryParams
	selector.Levels = nil
	base := buildLogQL(&selector)

	want := NormalizedLevels(p.Levels)
	// INFO is the remainder, so selecting it needs the total and all three of
	// the levels that are subtracted from it, selected or not.
	needInfo := want == nil || want["INFO"]
	need := func(level string) bool { return needInfo || want[level] }

	var (
		mu     sync.Mutex
		total  []int64
		counts = map[string][]int64{}
	)
	g, gctx := errgroup.WithContext(ctx)
	if needInfo {
		g.Go(func() error {
			series, err := c.countOverTime(gctx, base, start, end, step, buckets)
			if err != nil {
				return err
			}
			mu.Lock()
			defer mu.Unlock()
			total = series
			return nil
		})
	}
	for _, level := range []string{"ERROR", "WARN", "DEBUG"} {
		if !need(level) {
			continue
		}
		query := base + fmt.Sprintf(" |~ %q", levelLineMatch(level))
		g.Go(func() error {
			series, err := c.countOverTime(gctx, query, start, end, step, buckets)
			if err != nil {
				return err
			}
			mu.Lock()
			defer mu.Unlock()
			counts[level] = series
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return Histogram{}, fmt.Errorf("count log levels: %w", err)
	}

	out := emptyBuckets(start, buckets, step)
	for i := range out {
		out[i].Error = at(counts["ERROR"], i)
		out[i].Warn = at(counts["WARN"], i)
		out[i].Debug = at(counts["DEBUG"], i)
		if needInfo {
			// Clamped: a line carrying two severity shapes is counted twice
			// above, and a negative remainder would draw as a gap in the stack.
			out[i].Info = max(at(total, i)-out[i].Error-out[i].Warn-out[i].Debug, 0)
		}
	}
	h := Histogram{Buckets: out, Exact: true}
	zeroUnselectedLevels(&h, p.Levels)
	return h, nil
}

// at reads one bucket of a series that may not have been queried at all: a
// level the caller did not select is left nil rather than filled with zeros.
func at(series []int64, i int) int64 {
	if i >= len(series) {
		return 0
	}
	return series[i]
}

// countOverTime runs one count_over_time query_range and lays its samples out
// over the bucket series.
func (c *LokiClient) countOverTime(ctx context.Context, query string, start, end time.Time, step time.Duration, buckets int) ([]int64, error) {
	seconds := strconv.FormatInt(int64(step/time.Second), 10) + "s"
	metricQuery := "sum(count_over_time(" + query + "[" + seconds + "]))"

	u, err := url.Parse(c.baseURL + apiPrefix + "/query_range")
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	q := u.Query()
	q.Set("query", metricQuery)
	// The first sample sits one step in: count_over_time at time T covers
	// (T-step, T], so a sample at start would count the step *before* the
	// window.
	q.Set("start", strconv.FormatInt(start.Add(step).UnixNano(), 10))
	q.Set("end", strconv.FormatInt(end.UnixNano(), 10))
	q.Set("step", seconds)
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
		return nil, &StatusError{StatusCode: resp.StatusCode, Operation: "loki count_over_time"}
	}
	var result lokiMatrixResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode matrix response: %w", err)
	}
	if result.Status != "success" {
		return nil, fmt.Errorf("loki error: %s", result.Status)
	}

	out := make([]int64, buckets)
	for _, series := range result.Data.Result {
		for _, sample := range series.Values {
			ts, value, err := sample.parse()
			if err != nil {
				return nil, err
			}
			// A sample stamped T covers (T-step, T], which is bucket
			// (T-start)/step - 1. Samples outside the window are dropped
			// rather than clamped: folding them into an edge bucket would
			// invent a spike exactly where the eye looks for one.
			idx := int((ts.Sub(start)+step/2)/step) - 1
			if idx < 0 || idx >= buckets {
				continue
			}
			out[idx] += value
		}
	}
	return out, nil
}

// Loki HTTP API response envelope for query_range with a metric query.

type lokiMatrixResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string       `json:"resultType"`
		Result     []lokiMatrix `json:"result"`
	} `json:"data"`
}

type lokiMatrix struct {
	Metric map[string]string `json:"metric"`
	Values []lokiSample      `json:"values"`
}

// lokiSample is one [unixSeconds, "value"] pair. The timestamp is a JSON
// number and the value a quoted string, so the pair cannot decode into a
// single Go type.
type lokiSample [2]json.RawMessage

func (s *lokiSample) parse() (time.Time, int64, error) {
	var seconds float64
	if err := json.Unmarshal(s[0], &seconds); err != nil {
		return time.Time{}, 0, fmt.Errorf("parse sample timestamp %s: %w", s[0], err)
	}
	var raw string
	if err := json.Unmarshal(s[1], &raw); err != nil {
		return time.Time{}, 0, fmt.Errorf("parse sample value %s: %w", s[1], err)
	}
	// Loki renders counts as floats ("12" or "12.0"); they are whole by
	// construction, so a float parse keeps both shapes readable.
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("parse sample value %q: %w", raw, err)
	}
	return time.Unix(0, int64(seconds*float64(time.Second))), int64(value), nil
}
