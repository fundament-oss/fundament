// Package logs provides clients for querying container/application logs from a
// configured backend. Grafana Loki is the primary source; the Kubernetes
// pod-log endpoint (via the kube-api-proxy) is a narrower fallback used when
// Loki is not configured.
package logs

import (
	"context"
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
	Levels    []string
	Search    string
	Start     time.Time
	End       time.Time
	Limit     int
}

// Labels are the distinct label values available for a cluster.
type Labels struct {
	Namespaces []string
	Pods       []string
	Containers []string
}

// Client queries logs from a backend.
type Client interface {
	// Backend reports which concrete source this client targets.
	Backend() Backend
	// Query returns a bounded set of entries, newest first.
	Query(ctx context.Context, p QueryParams) ([]Entry, error)
	// Tail streams new entries until ctx is cancelled. The returned channel is
	// closed when the stream ends.
	Tail(ctx context.Context, p QueryParams) (<-chan Entry, error)
	// Labels returns distinct label values for filter dropdowns. namespace, when
	// non-empty, scopes pod/container results.
	Labels(ctx context.Context, clusterID, namespace string) (Labels, error)
}

const defaultLimit = 1000

// normalizeLevel maps a free-form severity string onto one of the four levels
// the UI understands, returning "" when it can't be classified.
func normalizeLevel(raw string) string {
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
