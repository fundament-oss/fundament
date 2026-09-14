package logs

import (
	"context"
	"fmt"
	"time"
)

// defaultBuckets is the histogram resolution used when a caller asks for none.
const defaultBuckets = 30

// MaxBuckets caps the requested bucket count. Every bucket is one aggregation
// step for the backend, and the count arrives from the wire; the proto carries
// the same ceiling, and this clamp guards every other path.
const MaxBuckets = 200

// EffectiveBuckets normalizes a caller-supplied bucket count into
// [1, MaxBuckets], falling back to the default when unset.
func EffectiveBuckets(buckets int) int {
	switch {
	case buckets <= 0:
		return defaultBuckets
	case buckets > MaxBuckets:
		return MaxBuckets
	default:
		return buckets
	}
}

// HistogramBucket is one time bucket, counted per normalised severity. Every
// bucket in a Histogram spans the same width, so it is not repeated here.
type HistogramBucket struct {
	Start time.Time
	Error int64
	Warn  int64
	Info  int64
	Debug int64
}

// add counts one entry of the given normalised level. An unrecognised level
// lands on Info, matching defaultLevel: the entry list reports exactly the
// same line as INFO, and a bucket that silently dropped it would not add up to
// what the reader can scroll through.
func (b *HistogramBucket) add(level string, n int64) {
	switch NormalizeLevel(level) {
	case "ERROR":
		b.Error += n
	case "WARN":
		b.Warn += n
	case "DEBUG":
		b.Debug += n
	default:
		b.Info += n
	}
}

// Histogram is the bucketed answer to a HistogramParams query.
type Histogram struct {
	// Buckets covers the whole requested window, oldest first, including the
	// empty ones: a gap in a chart has to be drawn as a zero, not as a missing
	// point.
	Buckets []HistogramBucket
	// Exact reports whether the counts cover the whole window. Backends that
	// can aggregate server-side set it; backends that can only count a bounded
	// page of entries (the Kubernetes pod-log fallback) clear it, and the
	// caller is expected to say so rather than present a sample as a total.
	Exact bool
}

// HistogramParams describes a bucketed count over a window. The selector
// fields are the query's, so the chart describes the same lines the entry list
// is showing.
//
// Limit bounds only the backends that cannot aggregate: it is how many entries
// they may read before counting them. A backend with a metric API ignores it
// and counts the whole window.
type HistogramParams struct {
	QueryParams
	// Buckets is the number of equal divisions of [Start, End).
	Buckets int
}

// window normalizes the requested range the way Query does, and returns the
// bucket count and width alongside it.
func (p *HistogramParams) window() (start, end time.Time, buckets int, step time.Duration) {
	end = p.End
	if end.IsZero() {
		end = time.Now()
	}
	start = p.Start
	if start.IsZero() {
		start = end.Add(-time.Hour)
	}
	buckets = EffectiveBuckets(p.Buckets)
	if !start.Before(end) {
		// A degenerate window still has to produce the requested series, so
		// the caller's chart keeps its shape; every bucket is empty.
		return start, end, buckets, 0
	}
	return start, end, buckets, end.Sub(start) / time.Duration(buckets)
}

// emptyBuckets lays out the series: one bucket per division, starting at
// start, each step wide.
func emptyBuckets(start time.Time, buckets int, step time.Duration) []HistogramBucket {
	out := make([]HistogramBucket, buckets)
	for i := range out {
		out[i] = HistogramBucket{Start: start.Add(time.Duration(i) * step)}
	}
	return out
}

// bucketEntries counts entries into the window's buckets. It is what a backend
// that cannot aggregate falls back to, so the result is only ever as complete
// as the entries handed to it — callers set Histogram.Exact accordingly.
func bucketEntries(entries []Entry, p *HistogramParams) Histogram {
	start, _, count, step := p.window()
	buckets := emptyBuckets(start, count, step)
	if step <= 0 {
		return Histogram{Buckets: buckets}
	}
	for i := range entries {
		idx := int(entries[i].Timestamp.Sub(start) / step)
		if idx < 0 || idx >= count {
			continue
		}
		buckets[idx].add(entries[i].Level, 1)
	}
	return Histogram{Buckets: buckets}
}

// zeroUnselectedLevels blanks the severities the caller did not ask for. The
// backends narrow approximately (see levelPreFilter), so this is the same
// exactness FilterByLevels gives the entry list: a level that was not
// requested reads as zero rather than leaking in through a generous match.
func zeroUnselectedLevels(h *Histogram, levels []string) {
	want := NormalizedLevels(levels)
	if want == nil {
		return
	}
	for i := range h.Buckets {
		b := &h.Buckets[i]
		if !want["ERROR"] {
			b.Error = 0
		}
		if !want["WARN"] {
			b.Warn = 0
		}
		if !want["INFO"] {
			b.Info = 0
		}
		if !want["DEBUG"] {
			b.Debug = 0
		}
	}
}

// histogramFromQuery counts a client's own bounded query into buckets. It is
// the fallback for backends with no aggregation API, and never claims to be
// exact: the entries it counts are the newest Limit lines, not the window.
//
// The limit is the caller's rather than MaxLimit, so a caller that pairs this
// with a query can ask for the same page twice over instead of paying for a
// second, larger read whose totals its entry list cannot account for.
func histogramFromQuery(ctx context.Context, c Client, p *HistogramParams) (Histogram, error) {
	qp := p.QueryParams
	qp.Limit = EffectiveLimit(qp.Limit)
	entries, err := c.Query(ctx, &qp)
	if err != nil {
		return Histogram{}, fmt.Errorf("histogram query: %w", err)
	}
	entries = FilterByLevels(entries, p.Levels)
	h := bucketEntries(entries, p)
	h.Exact = false
	return h, nil
}
