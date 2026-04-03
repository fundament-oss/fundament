package cluster_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	"github.com/fundament-oss/fundament/common/dbconst"
)

func TestCheckStatusTransitionCreatesEvent(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	// Insert cluster, sync it to create the shoot, then mark outbox completed.
	clusterID := insertCluster(t, db, acmeCorpOrgID, "status-ready")
	sc := handler.SyncContext{EntityType: handler.EntityCluster, Event: dbconst.ClusterOutboxEvent_Created, Source: dbconst.ClusterOutboxSource_Trigger}
	err := h.Sync(t.Context(), clusterID, sc)
	require.NoError(t, err)
	markOutboxCompleted(t, db, clusterID)

	// Set shoot_status_updated far enough in the past to bypass the 30s throttle.
	setShootStatus(t, db, clusterID, "progressing")

	// Mock returns Ready for this cluster (instant mock — shoot is immediately ready).
	err = h.CheckStatus(t.Context())
	require.NoError(t, err)

	// DB should have shoot_status = ready.
	status := getClusterShootStatus(t, db, clusterID)
	require.NotNil(t, status)
	require.Equal(t, "ready", *status)

	// status_ready event should exist.
	assertEventExists(t, db, clusterID, "status_ready")
}

func TestCheckStatusDeletedConfirmedGone(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	// Insert a soft-deleted cluster and mark outbox completed.
	clusterID := insertDeletedCluster(t, db, acmeCorpOrgID, "status-deleted")
	markOutboxCompleted(t, db, clusterID)

	// Set shoot_status_updated to bypass the throttle.
	// No shoot exists in mock — GetShootStatus will return {StatusPending, MsgShootNotFound}.
	setShootStatus(t, db, clusterID, "deleting")

	err := h.CheckStatus(t.Context())
	require.NoError(t, err)

	// DB should have shoot_status = deleted.
	status := getClusterShootStatus(t, db, clusterID)
	require.NotNil(t, status)
	require.Equal(t, "deleted", *status)

	// status_deleted event should exist.
	assertEventExists(t, db, clusterID, "status_deleted")
}

func TestCheckStatusErrorTransition(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	// Insert cluster, sync it, mark completed.
	clusterID := insertCluster(t, db, acmeCorpOrgID, "status-error")
	sc := handler.SyncContext{EntityType: handler.EntityCluster, Event: dbconst.ClusterOutboxEvent_Created, Source: dbconst.ClusterOutboxSource_Trigger}
	err := h.Sync(t.Context(), clusterID, sc)
	require.NoError(t, err)
	markOutboxCompleted(t, db, clusterID)
	setShootStatus(t, db, clusterID, "progressing")

	// Override mock to return error status.
	mock.SetStatusOverride(clusterID, gardener.StatusError, "shoot reconciliation failed")

	err = h.CheckStatus(t.Context())
	require.NoError(t, err)

	status := getClusterShootStatus(t, db, clusterID)
	require.NotNil(t, status)
	require.Equal(t, "error", *status)

	assertEventExists(t, db, clusterID, "status_error")
}
