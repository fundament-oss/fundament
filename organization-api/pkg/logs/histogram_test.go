package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// levelNeedle is the part of a level's line filter that survives %q quoting
// intact, so a test server can tell the four queries apart.
func levelNeedle(level string) string { return klogLevelLetter[level] + "[0-9]{4}" }

// matrixServer answers count_over_time queries with one sample per step,
// recording the LogQL it was asked. counts maps a substring of the query onto
// the value every sample carries, so a test can give each level its own rate.
func matrixServer(t *testing.T, counts map[string]int64) (*httptest.Server, *[]string) {
	t.Helper()
	var (
		mu      sync.Mutex
		queries []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		query := q.Get("query")
		mu.Lock()
		queries = append(queries, query)
		mu.Unlock()

		value := counts["total"]
		for needle, v := range counts {
			if needle != "total" && strings.Contains(query, needle) {
				value = v
			}
		}

		start, err := parseNano(q.Get("start"))
		require.NoError(t, err)
		end, err := parseNano(q.Get("end"))
		require.NoError(t, err)
		step, err := time.ParseDuration(q.Get("step"))
		require.NoError(t, err)

		var values [][2]any
		for ts := start; !ts.After(end); ts = ts.Add(step) {
			values = append(values, [2]any{float64(ts.UnixNano()) / float64(time.Second), fmt.Sprintf("%d", value)})
		}
		body := map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "matrix",
				"result":     []any{map[string]any{"metric": map[string]string{}, "values": values}},
			},
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &queries
}

func parseNano(s string) (time.Time, error) {
	var nano int64
	if _, err := fmt.Sscanf(s, "%d", &nano); err != nil {
		return time.Time{}, fmt.Errorf("parse nanos %q: %w", s, err)
	}
	return time.Unix(0, nano), nil
}

// The whole point of the RPC: counts come from the backend's aggregation over
// the window, so they are not bounded by MaxLimit the way an entry page is.
func TestLokiClient_HistogramCountsWholeWindow(t *testing.T) {
	srv, queries := matrixServer(t, map[string]int64{
		"total":              1000,
		levelNeedle("ERROR"): 7,
		levelNeedle("WARN"):  3,
		levelNeedle("DEBUG"): 90,
	})
	c := NewLokiClient(srv.URL)

	end := time.Now().Truncate(time.Second)
	h, err := c.Histogram(context.Background(), &HistogramParams{
		QueryParams: QueryParams{ClusterID: "c", Namespace: "kube-system", Start: end.Add(-30 * time.Minute), End: end},
		Buckets:     30,
	})
	require.NoError(t, err)
	require.Len(t, h.Buckets, 30)
	assert.True(t, h.Exact)

	var total int64
	for _, b := range h.Buckets {
		total += b.Error + b.Warn + b.Info + b.Debug
	}
	// 30 buckets x 1000 lines is six times MaxLimit: a client-side count over
	// an entry page could not have produced this number.
	assert.Greater(t, total, int64(MaxLimit))
	assert.Equal(t, int64(30*1000), total)

	for _, b := range h.Buckets {
		assert.Equal(t, int64(7), b.Error)
		assert.Equal(t, int64(3), b.Warn)
		assert.Equal(t, int64(90), b.Debug)
		// INFO is the remainder, which is what keeps the stack adding up to
		// the true total even where the per-level match is approximate.
		assert.Equal(t, int64(900), b.Info)
	}

	require.Len(t, *queries, 4, "one total plus one per directly counted level")
	for _, q := range *queries {
		assert.True(t, strings.HasPrefix(q, "sum(count_over_time("), q)
		assert.Contains(t, q, `namespace_name="kube-system"`)
		assert.Contains(t, q, "[60s]))", "30 buckets over 30 minutes is a 60s step")
	}
}

// A level the caller did not select reads as zero, and no query is spent on it.
func TestLokiClient_HistogramSkipsUnselectedLevels(t *testing.T) {
	srv, queries := matrixServer(t, map[string]int64{"total": 100, levelNeedle("ERROR"): 5})
	c := NewLokiClient(srv.URL)

	h, err := c.Histogram(context.Background(), &HistogramParams{
		QueryParams: QueryParams{ClusterID: "c", Levels: []string{"ERROR"}},
		Buckets:     4,
	})
	require.NoError(t, err)
	require.Len(t, h.Buckets, 4)
	for _, b := range h.Buckets {
		assert.Equal(t, int64(5), b.Error)
		assert.Zero(t, b.Warn)
		assert.Zero(t, b.Info)
		assert.Zero(t, b.Debug)
	}
	require.Len(t, *queries, 1, "only ERROR was asked for; the total is only needed to derive INFO")
	assert.Contains(t, (*queries)[0], levelNeedle("ERROR"))
}

// Selecting INFO still needs the other three: it is the remainder.
func TestLokiClient_HistogramInfoNeedsTotalAndSubtrahends(t *testing.T) {
	srv, queries := matrixServer(t, map[string]int64{
		"total":              10,
		levelNeedle("ERROR"): 1,
		levelNeedle("WARN"):  2,
		levelNeedle("DEBUG"): 3,
	})
	c := NewLokiClient(srv.URL)

	h, err := c.Histogram(context.Background(), &HistogramParams{
		QueryParams: QueryParams{ClusterID: "c", Levels: []string{"INFO"}},
		Buckets:     2,
	})
	require.NoError(t, err)
	assert.Len(t, *queries, 4)
	for _, b := range h.Buckets {
		assert.Equal(t, int64(4), b.Info, "10 total minus 1+2+3 counted elsewhere")
		assert.Zero(t, b.Error)
		assert.Zero(t, b.Warn)
		assert.Zero(t, b.Debug)
	}
}

// A line matching two severity shapes must not push INFO below zero: a negative
// remainder would draw as a gap in the stack.
func TestLokiClient_HistogramClampsOvercountedRemainder(t *testing.T) {
	srv, _ := matrixServer(t, map[string]int64{
		"total":              5,
		levelNeedle("ERROR"): 4,
		levelNeedle("WARN"):  4,
		levelNeedle("DEBUG"): 4,
	})
	c := NewLokiClient(srv.URL)

	h, err := c.Histogram(context.Background(), &HistogramParams{
		QueryParams: QueryParams{ClusterID: "c"},
		Buckets:     3,
	})
	require.NoError(t, err)
	for _, b := range h.Buckets {
		assert.Zero(t, b.Info)
	}
}

// Buckets tile the window end-first, oldest bucket first, each one step wide.
func TestLokiClient_HistogramBucketLayout(t *testing.T) {
	srv, _ := matrixServer(t, map[string]int64{"total": 1})
	c := NewLokiClient(srv.URL)

	end := time.Now().Truncate(time.Second)
	start := end.Add(-time.Hour)
	h, err := c.Histogram(context.Background(), &HistogramParams{
		QueryParams: QueryParams{ClusterID: "c", Start: start, End: end},
		Buckets:     12,
	})
	require.NoError(t, err)
	require.Len(t, h.Buckets, 12)
	assert.Equal(t, start.UTC(), h.Buckets[0].Start.UTC())
	for i := range h.Buckets {
		assert.Equal(t, start.Add(time.Duration(i)*5*time.Minute).UTC(), h.Buckets[i].Start.UTC())
	}
}

// The LogQL level filter and parseLogLine must classify the same line the same
// way. Nothing re-checks the counts the way FilterByLevels re-checks entries,
// so a drift here is a silently wrong chart.
func TestLevelLineMatchAgreesWithParseLogLine(t *testing.T) {
	lines := []string{
		`E0804 12:33:01.123456       1 reflector.go:1] failed to sync cache`,
		`F0804 12:33:01.123456       1 main.go:9] fatal: giving up`,
		`W0804 12:33:01.123456       1 kubelet.go:22] eviction threshold met`,
		`I0804 12:33:01.123456       1 server.go:3] serving on :8080`,
		`D0804 12:33:01.123456       1 cache.go:7] cache warm`,
		`{"level":"error","msg":"connection refused"}`,
		`{"level": "ERROR", "msg": "connection refused"}`,
		`{"severity":"warning","msg":"issuer not ready"}`,
		`{"lvl":"debug","msg":"xDS snapshot pushed"}`,
		`{"log_level":"notice","msg":"reloaded"}`,
		`{"level":"fatal","msg":"cannot bind"}`,
		`{"msg":"no level at all"}`,
		`plain text with no severity anywhere`,
		`{"level":30,"msg":"numeric level"}`,
	}

	patterns := map[string]*regexp.Regexp{}
	for _, level := range []string{"ERROR", "WARN", "DEBUG"} {
		patterns[level] = regexp.MustCompile(levelLineMatch(level))
	}

	for _, line := range lines {
		_, raw, _ := parseLogLine(line)
		want := NormalizeLevel(raw)
		if want == "" {
			// Unclassifiable lines are reported as defaultLevel by the entry
			// list, and counted into INFO as the remainder — so no pattern may
			// claim them.
			want = defaultLevel
		}
		for level, re := range patterns {
			matched := re.MatchString(line)
			assert.Equal(t, want == level, matched,
				"line %q: parseLogLine says %s, %s pattern match=%v", line, want, level, matched)
		}
	}
}

// The mock backend has to honour the same contract, or local dev and CI would
// exercise a chart shape production never produces.
func TestMockClient_HistogramCoversWindowBeyondEntryLimit(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	m := &MockClient{now: func() time.Time { return now }}

	end := now
	start := end.Add(-24 * time.Hour)
	h, err := m.Histogram(context.Background(), &HistogramParams{
		QueryParams: QueryParams{ClusterID: "c", Start: start, End: end},
		Buckets:     24,
	})
	require.NoError(t, err)
	require.Len(t, h.Buckets, 24)
	assert.True(t, h.Exact)

	var total int64
	for _, b := range h.Buckets {
		total += b.Error + b.Warn + b.Info + b.Debug
	}
	// 24h at one entry per mockInterval is far past the entry limit, which is
	// exactly the cliff the old client-side count drew.
	assert.Greater(t, total, int64(MaxLimit))
	assert.InDelta(t, float64(24*time.Hour/mockInterval), float64(total), 2)

	// Every bucket carries traffic: the point of the fix is that the window is
	// covered evenly rather than trailing off where the page ran out.
	for i, b := range h.Buckets {
		assert.Positive(t, b.Error+b.Warn+b.Info+b.Debug, "bucket %d is empty", i)
	}
}

// A level filter narrows the mock histogram the same way it narrows entries.
func TestMockClient_HistogramHonoursLevelFilter(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	m := &MockClient{now: func() time.Time { return now }}

	h, err := m.Histogram(context.Background(), &HistogramParams{
		QueryParams: QueryParams{ClusterID: "c", Start: now.Add(-time.Hour), End: now, Levels: []string{"ERROR"}},
		Buckets:     6,
	})
	require.NoError(t, err)
	var warn, info, debug int64
	for _, b := range h.Buckets {
		warn += b.Warn
		info += b.Info
		debug += b.Debug
	}
	assert.Zero(t, warn)
	assert.Zero(t, info)
	assert.Zero(t, debug)
}

// The pod-log fallback cannot aggregate, so it must say so rather than present
// a bounded page as a total.
func TestKubeClient_HistogramIsInexact(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("2026-09-10T12:00:00.000000000Z E0910 12:00:00.000000       1 x.go:1] boom\n"))
	}))
	defer srv.Close()

	c := NewKubeClient(srv.URL, http.Header{})
	end := time.Date(2026, 9, 10, 12, 30, 0, 0, time.UTC)
	h, err := c.Histogram(context.Background(), &HistogramParams{
		QueryParams: QueryParams{ClusterID: "c", Namespace: "plugin-x", Pod: "p", Start: end.Add(-time.Hour), End: end},
		Buckets:     6,
	})
	require.NoError(t, err)
	assert.False(t, h.Exact, "a counted page is not a counted window")
	require.Len(t, h.Buckets, 6)
}

// The fallback counts the caller's page, not MaxLimit's worth. A caller that
// pairs a histogram with a query passes the query's limit, and the chart then
// describes exactly the entries the list is showing instead of a larger read
// whose totals the list below it cannot account for.
func TestKubeClient_HistogramCountsTheCallersPage(t *testing.T) {
	end := time.Date(2026, 9, 10, 12, 30, 0, 0, time.UTC)
	var lines strings.Builder
	for i := range 20 {
		at := end.Add(-time.Duration(20-i) * time.Minute)
		fmt.Fprintf(&lines, "%s E0910 12:00:00.000000       1 x.go:1] boom %d\n", at.Format(time.RFC3339Nano), i)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(lines.String()))
	}))
	defer srv.Close()

	c := NewKubeClient(srv.URL, http.Header{})
	h, err := c.Histogram(context.Background(), &HistogramParams{
		QueryParams: QueryParams{
			ClusterID: "c",
			Namespace: "plugin-x",
			Pod:       "p",
			Start:     end.Add(-time.Hour),
			End:       end,
			Limit:     5,
		},
		Buckets: 6,
	})
	require.NoError(t, err)

	var total int64
	for _, b := range h.Buckets {
		total += b.Error + b.Warn + b.Info + b.Debug
	}
	assert.Equal(t, int64(5), total, "the histogram counted past the caller's limit")
	assert.False(t, h.Exact)
}

// A window wider than the mock can scan is under-counted at its old end, so it
// must not be reported as exact: that is the claim the RPC exists to stop the
// console making.
func TestMockClient_HistogramInexactPastScanCap(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	m := &MockClient{now: func() time.Time { return now }}

	end := now
	start := end.Add(-2 * mockMaxScan * mockInterval)
	h, err := m.Histogram(context.Background(), &HistogramParams{
		QueryParams: QueryParams{ClusterID: "c", Start: start, End: end},
		Buckets:     10,
	})
	require.NoError(t, err)
	require.Len(t, h.Buckets, 10)
	assert.False(t, h.Exact, "a scan that hit its cap did not cover the window")

	// The window it did reach is still counted, so the chart is a truthful
	// picture of the recent half rather than an empty one.
	assert.Positive(t, h.Buckets[len(h.Buckets)-1].Error+h.Buckets[len(h.Buckets)-1].Warn+
		h.Buckets[len(h.Buckets)-1].Info+h.Buckets[len(h.Buckets)-1].Debug)
	assert.Zero(t, h.Buckets[0].Error+h.Buckets[0].Warn+h.Buckets[0].Info+h.Buckets[0].Debug)
}

// No backend configured still has to return the series the caller asked for,
// or the chart loses its axis instead of showing an empty window.
func TestStubClient_HistogramReturnsEmptySeries(t *testing.T) {
	end := time.Now()
	h, err := StubClient{}.Histogram(context.Background(), &HistogramParams{
		QueryParams: QueryParams{Start: end.Add(-time.Hour), End: end},
		Buckets:     10,
	})
	require.NoError(t, err)
	require.Len(t, h.Buckets, 10)
	assert.True(t, h.Exact)
}
