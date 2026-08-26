package logs

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The mock preallocates on the limit, so an unbounded caller-supplied value
// used to be an out-of-memory vector rather than a big response.
func TestMockClientQueryClampsLimit(t *testing.T) {
	fixed := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	m := &MockClient{now: func() time.Time { return fixed }}

	entries, err := m.Query(context.Background(), &QueryParams{
		ClusterID: "cluster-1",
		Start:     fixed.Add(-30 * 24 * time.Hour),
		End:       fixed,
		Limit:     math.MaxInt32,
	})
	require.NoError(t, err)
	assert.LessOrEqual(t, len(entries), MaxLimit)
}

// A Search that matches nothing never fills the result set, so before the tick
// budget the loop ran once per mockInterval across a caller-supplied range — a
// zero-value Start meant ~2e10 iterations from one request.
func TestMockClientQueryBoundsScanOnUnmatchedSearch(t *testing.T) {
	fixed := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	m := &MockClient{now: func() time.Time { return fixed }}

	done := make(chan struct{})
	go func() {
		defer close(done)
		entries, err := m.Query(context.Background(), &QueryParams{
			ClusterID: "cluster-1",
			// The zero Start the proto happily accepts: year 1.
			Start:  time.Time{}.Add(time.Second),
			End:    fixed,
			Search: "no-entry-contains-this",
		})
		require.NoError(t, err)
		assert.Empty(t, entries)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Query did not return: the tick scan is unbounded")
	}
}

func TestMockClientQueryHonoursCancellation(t *testing.T) {
	fixed := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	m := &MockClient{now: func() time.Time { return fixed }}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := m.Query(ctx, &QueryParams{
		ClusterID: "cluster-1",
		Start:     fixed.Add(-30 * 24 * time.Hour),
		End:       fixed,
		Search:    "no-entry-contains-this",
	})
	require.ErrorIs(t, err, context.Canceled)
}

func TestMockClientQueryEmptyOnInvertedRange(t *testing.T) {
	fixed := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	m := &MockClient{now: func() time.Time { return fixed }}

	entries, err := m.Query(context.Background(), &QueryParams{
		ClusterID: "cluster-1",
		Start:     fixed,
		End:       fixed.Add(-time.Hour),
	})
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestMockClientQueryDeterministic(t *testing.T) {
	fixed := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	m := &MockClient{now: func() time.Time { return fixed }}

	p := QueryParams{ClusterID: "11111111-1111-1111-1111-111111111111", Limit: 50}
	first, err := m.Query(context.Background(), &p)
	require.NoError(t, err)
	second, err := m.Query(context.Background(), &p)
	require.NoError(t, err)

	require.Len(t, first, 50)
	assert.Equal(t, first, second)
	// Newest first, spaced by the mock interval.
	assert.True(t, first[0].Timestamp.After(first[1].Timestamp))
	assert.Equal(t, mockInterval, first[0].Timestamp.Sub(first[1].Timestamp))
}

func TestMockClientQueryDiffersPerCluster(t *testing.T) {
	fixed := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	m := &MockClient{now: func() time.Time { return fixed }}

	a, err := m.Query(context.Background(), &QueryParams{ClusterID: "11111111-1111-1111-1111-111111111111", Limit: 10})
	require.NoError(t, err)
	b, err := m.Query(context.Background(), &QueryParams{ClusterID: "22222222-2222-2222-2222-222222222222", Limit: 10})
	require.NoError(t, err)
	assert.NotEqual(t, a, b)
}

func TestMockClientQueryFilters(t *testing.T) {
	fixed := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	m := &MockClient{now: func() time.Time { return fixed }}

	entries, err := m.Query(context.Background(), &QueryParams{
		ClusterID: "11111111-1111-1111-1111-111111111111",
		Namespace: "kube-system",
		Limit:     20,
	})
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	for _, e := range entries {
		assert.Equal(t, "kube-system", e.Namespace)
	}

	none, err := m.Query(context.Background(), &QueryParams{
		ClusterID: "11111111-1111-1111-1111-111111111111",
		Namespace: "does-not-exist",
	})
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestMockClientLabels(t *testing.T) {
	m := NewMockClient()

	all, err := m.Labels(context.Background(), "c", "", time.Time{}, time.Time{})
	require.NoError(t, err)
	assert.Contains(t, all.Namespaces, "kube-system")
	assert.Contains(t, all.Namespaces, "plugin-envoy-gateway")
	assert.NotEmpty(t, all.Pods)

	scoped, err := m.Labels(context.Background(), "c", "monitoring", time.Time{}, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, []string{"node-exporter-vw6p8"}, scoped.Pods)
	assert.Equal(t, []string{"node-exporter"}, scoped.Containers)
	// Namespace list stays global so the dropdown can switch scope.
	assert.Contains(t, scoped.Namespaces, "kube-system")
}

func TestMockClientBackend(t *testing.T) {
	assert.Equal(t, BackendLoki, NewMockClient().Backend())
}
