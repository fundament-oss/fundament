package organization_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

// The cluster's sync state is its own latest sync row; a node pool's row never
// stands in for it, and a node pool whose own sync failed is named separately.
func Test_Cluster_SyncState_IsTheClustersOwnRow(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()
	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{ID: userID, Name: "test-user", OrgIDs: []uuid.UUID{orgID}}),
	)
	token := env.createAuthnToken(t, userID)
	client := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)

	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	createRes, err := client.CreateCluster(ctx, organizationv1.CreateClusterRequest_builder{
		Name: "old-version", Region: "eu-west-1", KubernetesVersion: "1.28",
	}.Build())
	require.NoError(t, err)
	clusterID := createRes.GetClusterId()

	// A node pool; its trigger queues a sync row of its own.
	_, err = env.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.node_pools (cluster_id, name, machine_type, autoscale_min, autoscale_max)
		 VALUES ($1, 'workers', 'm1.small', 1, 3)`, clusterID)
	require.NoError(t, err)

	syncState := func() *organizationv1.SyncState {
		res, err := client.GetCluster(ctx, organizationv1.GetClusterRequest_builder{ClusterId: clusterID}.Build())
		require.NoError(t, err)
		return res.GetCluster().GetSyncState()
	}
	setClusterRow := func(status, info string) {
		_, err := env.adminPool.Exec(t.Context(),
			`UPDATE tenant.cluster_outbox SET status = $2, status_info = $3 WHERE cluster_id = $1`, clusterID, status, info)
		require.NoError(t, err)
	}
	setNodePoolRow := func(status, info string) {
		_, err := env.adminPool.Exec(t.Context(),
			`UPDATE tenant.cluster_outbox SET status = $2, status_info = $3
			 WHERE node_pool_id = (SELECT tenant.node_pools.id FROM tenant.node_pools WHERE tenant.node_pools.cluster_id = $1)`,
			clusterID, status, info)
		require.NoError(t, err)
	}

	// The node pool waits on its precondition: pending with the reason, which
	// is not an error, and not the cluster's state.
	setNodePoolRow("pending", "parent cluster not synced to Gardener")
	state := syncState()
	assert.Equal(t, "pending", state.GetOutboxStatus())
	assert.False(t, state.HasOutboxError(), "a pending row's status_info is not an error")
	assert.False(t, state.HasFailedNodePoolName())

	// The cluster's own sync fails for good: that is what shows, with its own error.
	setClusterRow("failed", `Unsupported value: "1.31.0"`)
	state = syncState()
	assert.Equal(t, "failed", state.GetOutboxStatus())
	assert.Equal(t, `Unsupported value: "1.31.0"`, state.GetOutboxError())
	assert.False(t, state.HasFailedNodePoolName(), "the node pool is still waiting, not failed")

	// A node pool whose own sync failed is named next to the cluster's state.
	setNodePoolRow("failed", "machine type m1.small is not offered")
	state = syncState()
	assert.Equal(t, "failed", state.GetOutboxStatus())
	assert.Equal(t, `Unsupported value: "1.31.0"`, state.GetOutboxError(), "the cluster's own error stays")
	assert.Equal(t, "workers", state.GetFailedNodePoolName())
	assert.Equal(t, "machine type m1.small is not offered", state.GetFailedNodePoolError())

	// A newer, completed row for the node pool ends its failure.
	_, err = env.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.cluster_outbox (node_pool_id, event, source, status)
		 SELECT tenant.node_pools.id, 'updated', 'manual', 'completed' FROM tenant.node_pools WHERE tenant.node_pools.cluster_id = $1`,
		clusterID)
	require.NoError(t, err)
	state = syncState()
	assert.False(t, state.HasFailedNodePoolName())

	// A ready fan-out row for the cluster is not a sync; the failed sync still shows.
	_, err = env.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.cluster_outbox (cluster_id, event, source, status) VALUES ($1, 'ready', 'status', 'completed')`, clusterID)
	require.NoError(t, err)
	state = syncState()
	assert.Equal(t, "failed", state.GetOutboxStatus())

	// The same state through the list.
	listRes, err := client.ListClusters(ctx, organizationv1.ListClustersRequest_builder{}.Build())
	require.NoError(t, err)
	require.Len(t, listRes.GetClusters(), 1)
	assert.Equal(t, "failed", listRes.GetClusters()[0].GetSyncState().GetOutboxStatus())
	assert.Equal(t, `Unsupported value: "1.31.0"`, listRes.GetClusters()[0].GetSyncState().GetOutboxError())
}
