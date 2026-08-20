package logs

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"time"
)

// MockClient generates deterministic synthetic log entries so the console's
// log pages render in local dev and CI without any real backend. It presents
// itself as BackendLoki so the frontend exercises its full-featured code path.
type MockClient struct {
	now func() time.Time
}

// NewMockClient returns a mock backend using the wall clock.
func NewMockClient() *MockClient {
	return &MockClient{now: time.Now}
}

// mockStream is one synthetic log source; the mock emits entries round-robin
// across all streams that match the query filters.
type mockStream struct {
	namespace string
	pod       string
	container string
	level     string
	message   string
}

var mockStreams = []mockStream{
	{"kube-system", "coredns-565d847f94-k2xqb", "coredns", "INFO", "remote answer served from cache"},
	{"kube-system", "calico-node-dl58n", "calico-node", "INFO", "Using autodetected IPv4 address on interface eth0"},
	{"kube-system", "kube-proxy-9tqzr", "kube-proxy", "WARN", "clearing conntrack entries took longer than expected"},
	{"kube-system", "metrics-server-6f66c8d4b7-rp2ws", "metrics-server", "ERROR", `unable to fetch node metrics: node "worker-2" not found`},
	{"plugin-envoy-gateway", "envoy-gateway-7c9b6d5f4-x8jl2", "envoy-gateway", "INFO", "reconciled gateway listeners"},
	{"plugin-envoy-gateway", "envoy-gateway-7c9b6d5f4-x8jl2", "envoy-gateway", "DEBUG", "xDS snapshot pushed to 3 proxies"},
	{"plugin-cert-manager", "cert-manager-84f5b8c9d-tt4mn", "controller", "INFO", "certificate renewed successfully"},
	{"plugin-cert-manager", "cert-manager-84f5b8c9d-tt4mn", "controller", "WARN", "issuer not ready, requeueing"},
	{"monitoring", "node-exporter-vw6p8", "node-exporter", "INFO", "scrape completed"},
	{"monitoring", "node-exporter-vw6p8", "node-exporter", "ERROR", "collector netstat failed: read /proc/net/netstat: transient error"},
}

// mockInterval is the synthetic spacing between consecutive entries.
const mockInterval = 3 * time.Second

func (m *MockClient) Backend() Backend { return BackendLoki }

// Query synthesizes entries across the requested range, newest first, one
// entry per mockInterval, cycling deterministically through the matching
// streams. The cluster id seeds the cycle offset so different clusters show
// different (but stable) logs.
func (m *MockClient) Query(_ context.Context, p *QueryParams) ([]Entry, error) {
	end := p.End
	if end.IsZero() {
		end = m.now()
	}
	start := p.Start
	if start.IsZero() {
		start = end.Add(-time.Hour)
	}
	limit := EffectiveLimit(p.Limit)

	streams := matchingStreams(p)
	if len(streams) == 0 {
		return []Entry{}, nil
	}

	seed := hashSeed(p.ClusterID)
	// Anchor ticks to absolute time so repeated queries over the same range
	// return the same entries.
	tick := end.Truncate(mockInterval)
	entries := make([]Entry, 0, limit)
	for !tick.Before(start) && len(entries) < limit {
		idx := (tick.Unix()/int64(mockInterval.Seconds()) + seed) % int64(len(streams))
		e := m.entryAt(tick, p.ClusterID, &streams[idx])
		if p.Search == "" || strings.Contains(e.Message, p.Search) {
			entries = append(entries, e)
		}
		tick = tick.Add(-mockInterval)
	}
	return entries, nil
}

// Tail emits one synthetic entry per mockInterval until ctx is cancelled.
func (m *MockClient) Tail(ctx context.Context, p *QueryParams) (<-chan TailEvent, error) {
	streams := matchingStreams(p)
	ch := make(chan TailEvent)
	go func() {
		defer close(ch)
		if len(streams) == 0 {
			<-ctx.Done()
			return
		}
		seed := hashSeed(p.ClusterID)
		ticker := time.NewTicker(mockInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				idx := (now.Unix()/int64(mockInterval.Seconds()) + seed) % int64(len(streams))
				e := m.entryAt(now, p.ClusterID, &streams[idx])
				if p.Search != "" && !strings.Contains(e.Message, p.Search) {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case ch <- TailEvent{Entry: e}:
				}
			}
		}
	}()
	return ch, nil
}

// Labels lists the synthetic streams' label values.
func (m *MockClient) Labels(_ context.Context, _, namespace string, _, _ time.Time) (Labels, error) {
	var l Labels
	seenNS := map[string]bool{}
	seenPod := map[string]bool{}
	seenC := map[string]bool{}
	for _, st := range mockStreams {
		if !seenNS[st.namespace] {
			seenNS[st.namespace] = true
			l.Namespaces = append(l.Namespaces, st.namespace)
		}
		if namespace != "" && st.namespace != namespace {
			continue
		}
		if !seenPod[st.pod] {
			seenPod[st.pod] = true
			l.Pods = append(l.Pods, st.pod)
		}
		if !seenC[st.container] {
			seenC[st.container] = true
			l.Containers = append(l.Containers, st.container)
		}
	}
	return l, nil
}

func (m *MockClient) entryAt(ts time.Time, clusterID string, st *mockStream) Entry {
	return Entry{
		Timestamp: ts,
		Level:     st.level,
		Cluster:   clusterID,
		Namespace: st.namespace,
		Pod:       st.pod,
		Container: st.container,
		Message:   fmt.Sprintf("%s (seq=%d)", st.message, ts.Unix()/int64(mockInterval.Seconds())),
		Fields:    map[string]string{"source": "mock"},
	}
}

func matchingStreams(p *QueryParams) []mockStream {
	out := make([]mockStream, 0, len(mockStreams))
	for _, st := range mockStreams {
		if p.Namespace != "" && st.namespace != p.Namespace {
			continue
		}
		if p.Pod != "" && st.pod != p.Pod {
			continue
		}
		if p.Container != "" && st.container != p.Container {
			continue
		}
		out = append(out, st)
	}
	return out
}

// hashSeed maps a cluster id onto a small non-negative offset (fnv32a fits
// int64 without sign concerns).
func hashSeed(s string) int64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return int64(h.Sum32())
}
