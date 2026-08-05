package logs

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
