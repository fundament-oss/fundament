package cluster

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	"github.com/fundament-oss/fundament/common/kubename"
)

type fakePoolNodeReader struct {
	pools map[string][]shoot.PoolNode
	err   error
	calls int
}

func (f *fakePoolNodeReader) ListNodePools(context.Context, uuid.UUID) (map[string][]shoot.PoolNode, error) {
	f.calls++
	return f.pools, f.err
}

func handlerWithReader(r PoolNodeReader) *Handler {
	h := &Handler{logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))}
	h.SetPoolNodeReader(r)
	return h
}

func TestReadsPoolNodes(t *testing.T) {
	t.Parallel()

	reconciling := &gardener.ShootStatus{Status: gardener.StatusProgressing, Operation: gardener.OperationReconcile, APIServerAvailable: true}
	creating := &gardener.ShootStatus{Status: gardener.StatusProgressing, Operation: gardener.OperationCreate, APIServerAvailable: true}
	settled := []storedNodePool{{Name: "small", Status: "ready"}, {Name: "gone", Status: "error"}}

	tests := []struct {
		name     string
		stored   []storedNodePool
		observed *gardener.ShootStatus
		pushed   bool
		want     bool
	}{
		{
			name:     "a pool waits for its machines",
			stored:   []storedNodePool{{Name: "small", Status: "ready"}, {Name: "nomach", Status: "waiting"}},
			observed: reconciling,
			want:     true,
		},
		{
			name:     "not before the API server answers",
			stored:   []storedNodePool{{Name: "nomach", Status: "waiting"}},
			observed: &gardener.ShootStatus{Status: gardener.StatusProgressing, Operation: gardener.OperationCreate},
		},
		{
			name:     "not once every pool is settled",
			stored:   settled,
			observed: reconciling,
		},
		{
			name:     "the create runs until its Prometheus does, so every pool is read",
			stored:   settled,
			observed: creating,
			want:     true,
		},
		{
			name:     "a never-classified pool that a change of ours shapes",
			stored:   []storedNodePool{{Name: "new"}},
			observed: reconciling,
			pushed:   true,
			want:     true,
		},
		{
			name:   "a never-classified pool without a change: its counts could decide nothing",
			stored: []storedNodePool{{Name: "old"}},
			observed: &gardener.ShootStatus{
				Status: gardener.StatusError, Operation: gardener.OperationReconcile, APIServerAvailable: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, readsPoolNodes(tt.stored, tt.observed, tt.pushed))
		})
	}
}

func TestPoolNodesFor(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	stored := []storedNodePool{
		{Name: "small", Status: "progressing", Created: created},
		{Name: "nomach", Status: "waiting", Created: created},
	}

	t.Run("counts by worker name, own nodes since the pool was created", func(t *testing.T) {
		t.Parallel()
		r := &fakePoolNodeReader{pools: map[string][]shoot.PoolNode{
			kubename.GenerateWorkerName("small"): {
				{Created: created.Add(-time.Hour), Ready: true}, // the predecessor's
				{Created: created.Add(5 * time.Minute), Ready: true},
				{Created: created.Add(6 * time.Minute), Ready: false},
			},
		}}
		counts := handlerWithReader(r).poolNodesFor(t.Context(), uuid.New(), stored)
		assert.Equal(t, map[string]poolNodeCounts{
			"small":  {all: shoot.PoolNodes{Nodes: 3, Ready: 2}, own: shoot.PoolNodes{Nodes: 2, Ready: 1}},
			"nomach": {},
		}, counts)
		assert.Equal(t, map[string]shoot.PoolNodes{"small": {Nodes: 2, Ready: 1}, "nomach": {}}, ownNodeCounts(counts))
		assert.Equal(t, 1, r.calls)
	})

	t.Run("a failed read leaves the shoot-wide rules alone", func(t *testing.T) {
		t.Parallel()
		r := &fakePoolNodeReader{err: errors.New("forbidden")}
		counts := handlerWithReader(r).poolNodesFor(t.Context(), uuid.New(), stored)
		assert.Nil(t, counts)
		assert.Nil(t, ownNodeCounts(counts))
	})
}

func TestNodeCountChanged(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	counts := shoot.PoolNodes{Nodes: 2, Ready: 1}

	assert.True(t, nodeCountChanged(storedNodePool{}, counts, now), "nothing stored yet")
	assert.True(t, nodeCountChanged(storedNodePool{Nodes: &shoot.PoolNodes{Nodes: 2, Ready: 2}, NodesRead: now}, counts, now))
	assert.False(t, nodeCountChanged(storedNodePool{Nodes: &counts, NodesRead: now.Add(-30 * time.Second)}, counts, now))
	assert.True(t, nodeCountChanged(storedNodePool{Nodes: &counts, NodesRead: now.Add(-nodeCountMaxAge)}, counts, now),
		"an unchanged count is recorded again once it gets old")
}

func TestSoonest(t *testing.T) {
	t.Parallel()

	assert.Zero(t, soonest(0, 0))
	assert.Equal(t, time.Minute, soonest(0, time.Minute))
	assert.Equal(t, time.Minute, soonest(time.Minute, 0))
	assert.Equal(t, 30*time.Second, soonest(time.Minute, 30*time.Second))
	assert.Equal(t, 30*time.Second, soonest(30*time.Second, time.Minute))
}
