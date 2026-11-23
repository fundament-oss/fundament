package cluster_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	"github.com/fundament-oss/fundament/common/dbconst"
)

// A cluster deleted before it reached Gardener has no Shoot. When its delete
// sync fails for good, something must still record that there is nothing left
// to delete: until shoot_status is 'deleted' the cluster keeps its name taken
// and counts against the organization's cluster quota.
func TestDeletedNeverSyncedClusterIsMarkedDeletedAfterFailedDeleteSync(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	// No node pool: the cluster never reaches Gardener.
	clusterID := insertCluster(t, db, acmeCorpOrgID, "never-synced")
	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.clusters SET deleted = now() WHERE id = $1`, clusterID)
	require.NoError(t, err)

	// Gardener is down: the delete sync fails.
	mock.SetDeleteError(errors.New("gardener unreachable"))
	sc := handler.SyncContext{EntityType: handler.EntityCluster, Event: dbconst.ClusterOutboxEvent_Deleted, Source: dbconst.ClusterOutboxSource_Trigger}
	require.Error(t, h.Sync(t.Context(), clusterID, sc))
	// ... until the outbox gives up on the cluster's rows.
	_, err = db.adminPool.Exec(t.Context(),
		`UPDATE tenant.cluster_outbox SET status = 'failed', failed = now() WHERE cluster_id = $1`, clusterID)
	require.NoError(t, err)

	// Gardener is back. The status sweep and the reconcile loop both run.
	mock.SetDeleteError(nil)
	require.NoError(t, checkStatus(t, h))
	require.NoError(t, h.Reconcile(t.Context()))
	require.NoError(t, checkStatus(t, h))

	status := getClusterShootStatus(t, db, clusterID)
	require.NotNil(t, status, "a deleted cluster without a Shoot is recorded as deleted")
	assert.Equal(t, "deleted", *status)
}

// A status check may decide a pool's status from a shoot report read before a
// concurrent check recorded the cluster as lost and cleared its pools. That
// late write must not resurrect a pool status on a lost shoot.
func TestNodePoolStatusWriteIsRefusedAfterLost(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	queries := dbgen.New(db.workerPool)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "lost-pools")
	poolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-4", 1, 2)
	setShootState(t, db, clusterID, "lost", "Shoot disappeared", "", time.Minute)

	rows, err := queries.NodePoolUpdateStatus(t.Context(), dbgen.NodePoolUpdateStatusParams{
		NodePoolID: poolID,
		Status:     pgtype.Text{String: string(dbconst.NodePoolStatus_Ready), Valid: true},
		Message:    pgtype.Text{String: "machines are ready", Valid: true},
	})
	require.NoError(t, err)
	assert.Zero(t, rows, "a lost shoot's pools stay cleared")

	status, _ := getNodePoolStatus(t, db, clusterID, "workers")
	assert.Empty(t, status, "the pool keeps the cleared status")
}

// The check that reads the cluster as lost and finds its Shoot again writes
// its pools in that same check: the guard only refuses a lost the check did
// not read itself.
func TestNodePoolStatusWritesAfterRecoveryFromLost(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	queries := dbgen.New(db.workerPool)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "recovered-pools")
	poolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-4", 1, 2)
	setShootState(t, db, clusterID, "lost", "Shoot disappeared", "", time.Minute)

	rows, err := queries.NodePoolUpdateStatus(t.Context(), dbgen.NodePoolUpdateStatusParams{
		NodePoolID:    poolID,
		Status:        pgtype.Text{String: string(dbconst.NodePoolStatus_Ready), Valid: true},
		Message:       pgtype.Text{String: "machines are ready", Valid: true},
		ClusterStatus: "lost",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), rows)

	status, _ := getNodePoolStatus(t, db, clusterID, "workers")
	assert.Equal(t, string(dbconst.NodePoolStatus_Ready), status)
}
