package metal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	prom "github.com/fundament-oss/fundament/organization-api/pkg/prometheus"
)

// The labels as they arrive through the gateway collector: the exporter's own
// plus the scrape's job, namespace and pod.
const capacityBody = `{"status":"success","data":{"resultType":"vector","result":[` +
	`{"metric":{"__name__":"metal_partition_capacity_total","partition":"fire","size":"c1-large-x86","job":"metal-metrics-exporter","namespace":"metal-control-plane"},"value":[1764000000,"12"]},` +
	`{"metric":{"__name__":"metal_partition_capacity_allocated","partition":"fire","size":"c1-large-x86","job":"metal-metrics-exporter","namespace":"metal-control-plane"},"value":[1764000000,"8"]}]}}`

const emptyVector = `{"status":"success","data":{"resultType":"vector","result":[]}}`

const powerMatrix = `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[[1764000000,"4200"],[1764000300,"4300"]]}]}}`

type request struct {
	path  string
	query string
	start string
	end   string
	step  string
}

type fakeStore struct {
	mu       sync.Mutex
	requests []request
	capacity string
	status   int
}

func (f *fakeStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f.mu.Lock()
	f.requests = append(f.requests, request{
		path: r.URL.Path, query: q.Get("query"), start: q.Get("start"), end: q.Get("end"), step: q.Get("step"),
	})
	f.mu.Unlock()

	if f.status != 0 {
		w.WriteHeader(f.status)
		return
	}
	switch {
	case r.URL.Path == "/api/v1/query_range":
		_, _ = w.Write([]byte(powerMatrix))
	case strings.HasPrefix(q.Get("query"), `{__name__=~"metal_partition_capacity_`):
		_, _ = w.Write([]byte(f.capacity))
	default:
		_, _ = w.Write([]byte(emptyVector))
	}
}

func newFakeReader(t *testing.T, store *fakeStore, now time.Time) *Reader {
	t.Helper()
	srv := httptest.NewServer(store)
	t.Cleanup(srv.Close)
	r := NewReader(prom.NewHTTPClient(srv.URL))
	r.now = func() time.Time { return now }
	return r
}

func TestSnapshotAnswersEveryQueryByName(t *testing.T) {
	now := time.Date(2026, 11, 25, 10, 7, 31, 0, time.UTC)
	store := &fakeStore{capacity: capacityBody}
	snap, err := newFakeReader(t, store, now).Snapshot(context.Background())
	require.NoError(t, err)

	assert.Equal(t, now, snap.CollectedAt)
	require.Len(t, snap.Results, len(queries))
	for _, q := range queries {
		assert.Contains(t, snap.Results, q.name)
	}

	capacity := snap.Results[queryCapacity].Samples
	require.Len(t, capacity, 2)
	assert.Equal(t, "metal_partition_capacity_total", capacity[0].Labels["__name__"])
	assert.Equal(t, "fire", capacity[0].Labels["partition"])
	assert.InDelta(t, 12, capacity[0].Value, 0)

	power := snap.Results["power"].Series
	require.Len(t, power, 1)
	assert.Len(t, power[0].Samples, 2)
	assert.Empty(t, snap.Results["core_errors"].Samples, "no partition collector yet")
}

func TestSnapshotRangeQueriesUseFixedSteps(t *testing.T) {
	now := time.Date(2026, 11, 25, 10, 7, 31, 0, time.UTC)
	store := &fakeStore{capacity: capacityBody}
	_, err := newFakeReader(t, store, now).Snapshot(context.Background())
	require.NoError(t, err)

	byQuery := map[string]request{}
	for _, req := range store.requests {
		byQuery[req.query] = req
	}

	power := byQuery[`sum(metal_machine_power_usage)`]
	assert.Equal(t, "/api/v1/query_range", power.path)
	assert.Equal(t, "300", power.step)
	// Ends on the 5 min boundary before now, starts 24 h earlier.
	powerEnd := time.Date(2026, 11, 25, 10, 5, 0, 0, time.UTC)
	assert.Equal(t, strconv.FormatInt(powerEnd.Unix(), 10), power.end)
	assert.Equal(t, strconv.FormatInt(powerEnd.Add(-24*time.Hour).Unix(), 10), power.start)

	allocation := byQuery[`sum(metal_partition_capacity_allocated)`]
	assert.Equal(t, "3600", allocation.step)
	allocationEnd := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	assert.Equal(t, strconv.FormatInt(allocationEnd.Unix(), 10), allocation.end)
	assert.Equal(t, strconv.FormatInt(allocationEnd.Add(-7*24*time.Hour).Unix(), 10), allocation.start)

	capacity := byQuery[queries[0].expr]
	assert.Equal(t, "/api/v1/query", capacity.path)
}

func TestSnapshotWithoutCapacityIsNoData(t *testing.T) {
	store := &fakeStore{capacity: emptyVector}
	_, err := newFakeReader(t, store, time.Now()).Snapshot(context.Background())
	require.ErrorIs(t, err, ErrNoData)
}

func TestSnapshotStoreErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		bad    bool
	}{
		{name: "malformed query", status: http.StatusBadRequest, bad: true},
		{name: "unexecutable query", status: http.StatusUnprocessableEntity, bad: true},
		{name: "store down", status: http.StatusServiceUnavailable, bad: false},
		{name: "ingress refuses", status: http.StatusForbidden, bad: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{status: tt.status}
			_, err := newFakeReader(t, store, time.Now()).Snapshot(context.Background())
			require.Error(t, err)
			assert.Equal(t, tt.bad, IsBadQuery(err))
			assert.NotErrorIs(t, err, ErrNoData)
		})
	}
}

func TestSnapshotUnreachableStore(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	_, err := NewReader(prom.NewHTTPClient(url)).Snapshot(context.Background())
	require.Error(t, err)
	assert.False(t, IsBadQuery(err))
}
