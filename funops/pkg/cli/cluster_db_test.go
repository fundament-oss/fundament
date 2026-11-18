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
	assert.Len(t, queued, liveClusterCount(t, ctx))

	require.NoError(t, (&ClusterSyncCmd{All: true}).Run(ctx))
	assert.Len(t, pendingManualSyncs(t, ctx), len(queued))
}

// liveClusterCount counts the clusters that are not deleted, fixtures included.
func liveClusterCount(t *testing.T, ctx *Context) int {
	t.Helper()

	var count int
	require.NoError(t, ctx.DB.Pool.QueryRow(t.Context(), `SELECT count(*) FROM tenant.clusters WHERE deleted IS NULL`).Scan(&count))
	return count
}

// One cluster ID queues a sync of that cluster only.
func TestClusterSync_One(t *testing.T) {
	ctx := newTestContext(t)

	live := syncTestClusters(t, ctx)

	require.NoError(t, (&ClusterSyncCmd{Cluster: live[0].String()}).Run(ctx))
	assert.Equal(t, []uuid.UUID{live[0]}, pendingManualSyncs(t, ctx))
}

func TestClusterSync_Arguments(t *testing.T) {
	ctx := newTestContext(t)

	require.EqualError(t, (&ClusterSyncCmd{}).Run(ctx), "give a cluster ID or --all, not both")
	require.EqualError(t, (&ClusterSyncCmd{Cluster: uuid.NewString(), All: true}).Run(ctx), "give a cluster ID or --all, not both")
	require.EqualError(t, (&ClusterSyncCmd{Cluster: "nope"}).Run(ctx), `cluster ID "nope" is not a UUID`)
}
