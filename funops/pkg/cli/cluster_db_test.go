package cli

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pendingManualSyncs lists the clusters with a queued sync from funops.
func pendingManualSyncs(t *testing.T, ctx *Context) []uuid.UUID {
	t.Helper()

	rows, err := ctx.DB.Pool.Query(t.Context(), `
		SELECT cluster_id
		FROM tenant.cluster_outbox
		WHERE source = 'manual' AND event = 'updated' AND status = 'pending'`)
	require.NoError(t, err)
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	return ids
}

// syncTestClusters gives globex two live clusters and one deleted one, and
// settles the outbox rows the inserts queued, so only rows from the command
// under test are pending.
func syncTestClusters(t *testing.T, ctx *Context) (live []uuid.UUID) {
	t.Helper()

	const globex = "019b4000-0000-7000-8000-000000000002"
	_, err := ctx.DB.Pool.Exec(t.Context(), `UPDATE tenant.organizations SET quota_clusters = 5 WHERE id = $1`, globex)
	require.NoError(t, err)

	for _, name := range []string{"one", "two"} {
		var id uuid.UUID
		err := ctx.DB.Pool.QueryRow(t.Context(), `
			INSERT INTO tenant.clusters (organization_id, name, region, kubernetes_version)
			VALUES ($1, $2, 'local', '1.31')
			RETURNING id`, globex, name).Scan(&id)
		require.NoError(t, err)
		live = append(live, id)
	}
	_, err = ctx.DB.Pool.Exec(t.Context(), `
		INSERT INTO tenant.clusters (organization_id, name, region, kubernetes_version, deleted, shoot_status)
		VALUES ($1, 'gone', 'local', '1.31', now(), 'deleted')`, globex)
	require.NoError(t, err)

	_, err = ctx.DB.Pool.Exec(t.Context(), `UPDATE tenant.cluster_outbox SET status = 'completed'`)
	require.NoError(t, err)
	return live
}

// Every live cluster gets one queued sync; a second run adds none, and a
// deleted cluster gets none.
func TestClusterSync_All(t *testing.T) {
	ctx := newTestContext(t)
	live := syncTestClusters(t, ctx)

	require.NoError(t, (&ClusterSyncCmd{All: true}).Run(ctx))
	queued := pendingManualSyncs(t, ctx)
	assert.Subset(t, queued, live)
	assert.ElementsMatch(t, liveClusters(t, ctx), queued, "exactly the clusters that are not deleted")

	require.NoError(t, (&ClusterSyncCmd{All: true}).Run(ctx))
	assert.Len(t, pendingManualSyncs(t, ctx), len(queued))
}

// A cluster whose sync is waiting for a retry is not queued again.
func TestClusterSync_All_SkipsClusterWithSyncWaiting(t *testing.T) {
	ctx := newTestContext(t)
	live := syncTestClusters(t, ctx)

	_, err := ctx.DB.Pool.Exec(t.Context(), `
		INSERT INTO tenant.cluster_outbox (cluster_id, event, source, status)
		VALUES ($1, 'created', 'trigger', 'retrying')`, live[0])
	require.NoError(t, err)

	require.NoError(t, (&ClusterSyncCmd{All: true}).Run(ctx))
	queued := pendingManualSyncs(t, ctx)
	assert.NotContains(t, queued, live[0])
	assert.Contains(t, queued, live[1])
}

// liveClusters lists the clusters that are not deleted, fixtures included.
func liveClusters(t *testing.T, ctx *Context) []uuid.UUID {
	t.Helper()

	rows, err := ctx.DB.Pool.Query(t.Context(), `SELECT id FROM tenant.clusters WHERE deleted IS NULL`)
	require.NoError(t, err)
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	return ids
}

// One cluster ID queues a sync of that cluster only.
func TestClusterSync_One(t *testing.T) {
	ctx := newTestContext(t)

	live := syncTestClusters(t, ctx)

	require.NoError(t, (&ClusterSyncCmd{Cluster: live[0].String()}).Run(ctx))
	assert.Equal(t, []uuid.UUID{live[0]}, pendingManualSyncs(t, ctx))

	// A second run queues nothing, and neither does an unknown cluster: both
	// are errors, so the operator does not take them for a queued sync.
	err := (&ClusterSyncCmd{Cluster: live[0].String()}).Run(ctx)
	require.ErrorContains(t, err, "no sync queued for cluster "+live[0].String())
	unknown := uuid.NewString()
	err = (&ClusterSyncCmd{Cluster: unknown}).Run(ctx)
	require.ErrorContains(t, err, "no sync queued for cluster "+unknown)
}

func TestClusterSync_Arguments(t *testing.T) {
	ctx := newTestContext(t)

	require.EqualError(t, (&ClusterSyncCmd{}).Run(ctx), "give a cluster ID or --all")
	require.EqualError(t, (&ClusterSyncCmd{Cluster: uuid.NewString(), All: true}).Run(ctx), "give a cluster ID or --all, not both")
	require.EqualError(t, (&ClusterSyncCmd{Cluster: "nope"}).Run(ctx), `cluster ID "nope" is not a UUID`)
}
