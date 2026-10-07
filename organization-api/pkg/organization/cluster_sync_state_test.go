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
// stands in for it.
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
		 VALUES ($1, 'workers', 'n1-standard-1', 1, 3)`, clusterID)
	require.NoError(t, err)

	getCluster := func() *organizationv1.ClusterDetails {
		res, err := client.GetCluster(ctx, organizationv1.GetClusterRequest_builder{ClusterId: clusterID}.Build())
		require.NoError(t, err)
		return res.GetCluster()
	}
	syncState := func() *organizationv1.SyncState { return getCluster().GetSyncState() }
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
	assert.Equal(t, organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING, getCluster().GetStatus())

	// The cluster's own sync fails for good: that is what shows, with its own
	// error, and a cluster Gardener never got is in error.
	setClusterRow("failed", `Unsupported value: "1.31.0"`)
	state = syncState()
	assert.Equal(t, "failed", state.GetOutboxStatus())
	assert.Equal(t, `Unsupported value: "1.31.0"`, state.GetOutboxError())
	assert.Equal(t, organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR, getCluster().GetStatus())

	// A node pool whose own sync failed leaves the cluster's state alone.
	setNodePoolRow("failed", "machine type m1.small is not offered")
	state = syncState()
	assert.Equal(t, "failed", state.GetOutboxStatus())
	assert.Equal(t, `Unsupported value: "1.31.0"`, state.GetOutboxError(), "the cluster's own error stays")

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

// Each node pool reports its own latest sync, retrying ones included, so
// several broken pools show at once and a pending one is not an error.
func Test_NodePool_SyncState_IsEachPoolsOwn(t *testing.T) {
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
		Name: "pools", Region: "eu-west-1", KubernetesVersion: "1.28",
	}.Build())
	require.NoError(t, err)
	clusterID := createRes.GetClusterId()

	// Each pool's insert queues a sync row of its own.
	rows := map[string][2]string{
		"retrying": {"retrying", "quota exceeded, retrying"},
		"failed":   {"failed", "machine type not offered"},
		"waiting":  {"pending", "parent cluster not synced to Gardener"},
		"synced":   {"completed", ""},
	}
	for name, row := range rows {
		_, err = env.adminPool.Exec(t.Context(),
			`INSERT INTO tenant.node_pools (cluster_id, name, machine_type, autoscale_min, autoscale_max)
			 VALUES ($1, $2, 'n1-standard-1', 1, 3)`, clusterID, name)
		require.NoError(t, err)
		_, err = env.adminPool.Exec(t.Context(),
			`UPDATE tenant.cluster_outbox SET status = $3, status_info = NULLIF($4, '')
			 WHERE node_pool_id = (SELECT tenant.node_pools.id FROM tenant.node_pools
			                       WHERE tenant.node_pools.cluster_id = $1 AND tenant.node_pools.name = $2)`,
			clusterID, name, row[0], row[1])
		require.NoError(t, err)
	}

	listRes, err := client.ListNodePools(ctx, organizationv1.ListNodePoolsRequest_builder{ClusterId: clusterID}.Build())
	require.NoError(t, err)
	byName := map[string]*organizationv1.NodePool{}
	for _, pool := range listRes.GetNodePools() {
		byName[pool.GetName()] = pool
	}
	require.Len(t, byName, len(rows))

	assert.Equal(t, "retrying", byName["retrying"].GetSyncStatus())
	assert.Equal(t, "quota exceeded, retrying", byName["retrying"].GetSyncError())
	assert.Equal(t, "failed", byName["failed"].GetSyncStatus())
	assert.Equal(t, "machine type not offered", byName["failed"].GetSyncError())
	assert.Equal(t, "pending", byName["waiting"].GetSyncStatus())
	assert.False(t, byName["waiting"].HasSyncError(), "a pending row's status_info is not an error")
	assert.Equal(t, "completed", byName["synced"].GetSyncStatus())
	assert.False(t, byName["synced"].HasSyncError())

	// Read one by one, the same.
	getRes, err := client.GetNodePool(ctx, organizationv1.GetNodePoolRequest_builder{NodePoolId: byName["failed"].GetId()}.Build())
	require.NoError(t, err)
	assert.Equal(t, "machine type not offered", getRes.GetNodePool().GetSyncError())

	// A newer, completed sync of the failed pool ends its error.
	_, err = env.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.cluster_outbox (node_pool_id, event, source, status) VALUES ($1, 'updated', 'manual', 'completed')`,
		byName["failed"].GetId())
	require.NoError(t, err)
	getRes, err = client.GetNodePool(ctx, organizationv1.GetNodePoolRequest_builder{NodePoolId: byName["failed"].GetId()}.Build())
	require.NoError(t, err)
	assert.Equal(t, "completed", getRes.GetNodePool().GetSyncStatus())
	assert.False(t, getRes.GetNodePool().HasSyncError())
}
