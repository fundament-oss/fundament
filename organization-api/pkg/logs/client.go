// Package logs provides clients for querying container/application logs from a
// configured backend. Grafana Loki is the primary source; the Kubernetes
// pod-log endpoint (via the kube-api-proxy) is a narrower fallback used when
// Loki is not configured.
package logs

import (
	"context"
	"strings"
	"time"
)

// Backend identifies a concrete log source.
type Backend string

const (
	// BackendNone means no log backend is configured; queries return empty.
	BackendNone Backend = "none"
	// BackendLoki is Grafana Loki — full features.
	BackendLoki Backend = "loki"
	// BackendKubernetes is the Kubernetes pod-log endpoint via kube-api-proxy.
	BackendKubernetes Backend = "kubernetes"
)

// Entry is a single log line.
type Entry struct {
	Timestamp time.Time
	// Level normalised to ERROR / WARN / INFO / DEBUG; empty when unknown.
	Level     string
	Cluster   string
	Namespace string
	Pod       string
	Container string
	Message   string
	Fields    map[string]string
}

// QueryParams describes a bounded log query.
type QueryParams struct {
	ClusterID string
	Namespace string
	Pod       string
	Container string
	Search    string
	// Levels, when non-empty, restricts results to those normalised severities.
	// Backends narrow the query with it before applying Limit; FilterByLevels
	// then enforces it exactly.
	Levels []string
	Start  time.Time
	End    time.Time
	Limit  int
}

// Labels are the distinct label values available for a cluster.
type Labels struct {
	Namespaces []string
	Pods       []string
	Containers []string
}

// TailEvent is one item of a tail stream: either an entry, or the terminal
// error that ended it. A tail cannot report failure through its return value —
// it fails long after Tail returned — and a bare channel close is
// indistinguishable from a healthy end of stream, which left rotated
// credentials and dead backends looking like a quiet cluster.
type TailEvent struct {
	Entry Entry
	// Err, when non-nil, is the reason the stream ended. It is the last event
	// on the channel; no entry accompanies it.
	Err error
}

// Client queries logs from a backend.
type Client interface {
	// Backend reports which concrete source this client targets.
	Backend() Backend
	// Query returns a bounded set of entries, newest first.
	Query(ctx context.Context, p *QueryParams) ([]Entry, error)
	// Tail streams new entries until ctx is cancelled or the stream fails. The
	// returned channel is closed when the stream ends; a failure arrives as a
	// final TailEvent carrying Err.
	Tail(ctx context.Context, p *QueryParams) (<-chan TailEvent, error)
	// Labels returns distinct label values for filter dropdowns. namespace, when
	// non-empty, scopes pod/container results. start/end bound the window the
	// values are observed in (label values are time-scoped in Vali); zero
	// values leave the backend's default window.
	Labels(ctx context.Context, clusterID, namespace string, start, end time.Time) (Labels, error)
}

const defaultLimit = 1000

// defaultLevel is reported for an entry whose severity could not be classified.
// It is load-bearing for the level filter: because an unclassifiable line lands
// here, a query narrowing on this level cannot narrow at all (see
// levelPreFilter).
const defaultLevel = "INFO"

// MaxLimit caps a caller-supplied entry limit. Backends preallocate on the
// limit, so an unbounded value from the wire is an out-of-memory vector — the
// proto carries the same ceiling, and this clamp guards every other path.
const MaxLimit = 5000

// EffectiveLimit normalizes a caller-supplied limit into [1, MaxLimit],
// falling back to the default when unset.
func EffectiveLimit(limit int) int {
	switch {
	case limit <= 0:
		return defaultLimit
	case limit > MaxLimit:
		return MaxLimit
	default:
		return limit
	}
}

// NormalizedLevels maps the caller's requested severities onto the normalised
// set, dropping anything unrecognised. A nil result means "no level filter".
func NormalizedLevels(levels []string) map[string]bool {
	if len(levels) == 0 {
		return nil
	}
	out := make(map[string]bool, len(levels))
	for _, l := range levels {
		if n := NormalizeLevel(l); n != "" {
			out[n] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// FilterByLevels keeps only entries whose level is in levels, exactly. Backends
// narrow their queries approximately so that Limit is applied to a relevant set;
// this is what makes the filter precise, and it runs server-side so that every
// backend — and every API consumer, not just the console — sees one semantics.
func FilterByLevels(entries []Entry, levels []string) []Entry {
	want := NormalizedLevels(levels)
	if want == nil {
		return entries
	}
	out := make([]Entry, 0, len(entries))
	for i := range entries {
		if want[NormalizeLevel(entries[i].Level)] {
			out = append(out, entries[i])
		}
	}
	return out
}

// MatchesSearch reports whether a message satisfies a free-text filter. Matching
// is case-insensitive, which is what the console's filter box implies and what
// the Vali line filter now does — previously the backends matched
// case-sensitively while the client re-filtered case-insensitively, so searching
// "Timeout" on a service logging "timeout" returned nothing.
func MatchesSearch(message, search string) bool {
	if search == "" {
		return true
	}
	return strings.Contains(strings.ToLower(message), strings.ToLower(search))
}

// NormalizeLevel maps a free-form severity string onto one of the four levels
// the UI understands, returning "" when it can't be classified.
func NormalizeLevel(raw string) string {
	switch s := toUpperASCII(raw); {
	case s == "":
		return ""
	case hasAnyPrefix(s, "ERR", "FATAL", "CRIT", "PANIC", "EMERG", "ALERT"):
		return "ERROR"
	case hasAnyPrefix(s, "WARN"):
		return "WARN"
	case hasAnyPrefix(s, "DEBUG", "TRACE"):
		return "DEBUG"
	case hasAnyPrefix(s, "INFO", "NOTICE"):
		return "INFO"
	default:
		return ""
	}
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if len(s) >= len(p) && s[:len(p)] == p {
			return true
		}
	}
	return false
}

func toUpperASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - ('a' - 'A')
		}
	}
	return string(b)
}
