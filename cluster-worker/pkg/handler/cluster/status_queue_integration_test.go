package cluster_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
)

// A cluster queued several times before a worker takes it is checked once.
func TestStatusQueueDeduplicates(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := readyCluster(t, db, "queue-dedup")
	setShootState(t, db, clusterID, "progressing", "Create: Waiting", "", time.Minute)
	for range 3 {
		h.EnqueueStatusCheck(clusterID)
	}
	h.DrainStatusQueue(t.Context())

	assert.Equal(t, 1, mock.StatusCallsFor(clusterID))
}

// A failed check goes back on the queue with backoff instead of being dropped.
func TestStatusQueueRetriesFailedCheck(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := readyCluster(t, db, "queue-retry")
	setShootState(t, db, clusterID, "progressing", "Create: Waiting", "", time.Minute)
	mock.GetStatusError = assert.AnError

	h.EnqueueStatusCheck(clusterID)
	h.DrainStatusQueue(t.Context())

	assert.Equal(t, 1, h.StatusRequeues(clusterID))
}

// The workers process queued checks until the context ends, then return.
func TestRunStatusWorkers(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := readyCluster(t, db, "queue-workers")
	setShootState(t, db, clusterID, "progressing", "Create: Waiting", "", time.Minute)
	mock.SetShootStatusOverride(clusterID, gardener.StatusOverride{
		Status: gardener.StatusReady, Message: gardener.MsgShootReady, Operation: gardener.OperationCreate, Healthy: true,
	})

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- h.RunStatusWorkers(ctx) }()

	h.EnqueueStatusCheck(clusterID)
	assert.Eventually(t, func() bool {
		return countEvents(t, db, clusterID, "status_ready") == 1
	}, 10*time.Second, 50*time.Millisecond)

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("RunStatusWorkers did not return after the context ended")
	}
}

// A Shoot event for a cluster whose first sync has not completed yet is
// checked again shortly instead of being dropped.
func TestStatusQueueRetriesClusterNotSyncedYet(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "queue-unsynced")
	h.EnqueueStatusCheck(clusterID)
	h.DrainStatusQueue(t.Context())

	assert.Equal(t, 1, h.StatusRequeues(clusterID))
	assert.Equal(t, 0, mock.StatusCallsFor(clusterID))
}
