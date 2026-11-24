package organization_test

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

func requireNotFound(t *testing.T, err error) {
	t.Helper()
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// confirmShootDeleted writes what cluster-worker writes once Gardener confirms
// a deleted cluster's Shoot is gone.
func (q *quotaEnv) confirmShootDeleted(t *testing.T, clusterID string) {
	t.Helper()
	_, err := q.env.adminPool.Exec(t.Context(),
		`UPDATE tenant.clusters SET shoot_status = 'deleted' WHERE id = $1 AND deleted IS NOT NULL`, clusterID)
	require.NoError(t, err)
}

// A deleted cluster stays readable, DELETING, until Gardener confirms its
// Shoot is gone, so a caller can follow the deletion on the cluster itself.
func Test_Cluster_Deleting_ReadableUntilGardenerConfirms(t *testing.T) {
	t.Parallel()
	q := newQuotaEnv(t)
	q.setNodeQuota(t, offeringN1Standard1, 4)
	clusterID, err := q.createCluster("c")
	require.NoError(t, err)
	poolID, err := q.createNodePool(clusterID, "only", "n1-standard-1", 2)
	require.NoError(t, err)

	_, err = q.client.DeleteCluster(q.ctx, organizationv1.DeleteClusterRequest_builder{ClusterId: clusterID}.Build())
	require.NoError(t, err)

	got, err := q.client.GetCluster(q.ctx, organizationv1.GetClusterRequest_builder{ClusterId: clusterID}.Build())
	require.NoError(t, err)
	assert.Equal(t, organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING, got.GetCluster().GetStatus())

	byName, err := q.client.GetClusterByName(q.ctx, organizationv1.GetClusterByNameRequest_builder{Name: "c"}.Build())
	require.NoError(t, err)
	assert.Equal(t, clusterID, byName.GetCluster().GetId())
	assert.Equal(t, organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING, byName.GetCluster().GetStatus())

	// Asking again while it is being deleted is no error.
	_, err = q.client.DeleteCluster(q.ctx, organizationv1.DeleteClusterRequest_builder{ClusterId: clusterID}.Build())
	require.NoError(t, err)

	pools, err := q.client.ListNodePools(q.ctx, organizationv1.ListNodePoolsRequest_builder{ClusterId: clusterID}.Build())
	require.NoError(t, err)
	require.Len(t, pools.GetNodePools(), 1)
	assert.Equal(t, poolID, pools.GetNodePools()[0].GetId())
	_, err = q.client.GetNodePool(q.ctx, organizationv1.GetNodePoolRequest_builder{NodePoolId: poolID}.Build())
	require.NoError(t, err)

	q.confirmShootDeleted(t, clusterID)

	_, err = q.client.GetCluster(q.ctx, organizationv1.GetClusterRequest_builder{ClusterId: clusterID}.Build())
	requireNotFound(t, err)
	_, err = q.client.GetClusterByName(q.ctx, organizationv1.GetClusterByNameRequest_builder{Name: "c"}.Build())
	requireNotFound(t, err)
	_, err = q.client.ListNodePools(q.ctx, organizationv1.ListNodePoolsRequest_builder{ClusterId: clusterID}.Build())
	requireNotFound(t, err)
	_, err = q.client.DeleteCluster(q.ctx, organizationv1.DeleteClusterRequest_builder{ClusterId: clusterID}.Build())
	requireNotFound(t, err)
	_, err = q.client.GetNodePool(q.ctx, organizationv1.GetNodePoolRequest_builder{NodePoolId: poolID}.Build())
	requireNotFound(t, err)
	requireNotFound(t, q.deleteNodePool(poolID))
	assert.Equal(t, 1, q.livePools(t), "the pool goes with the cluster, not on its own")
}

// A cluster being deleted stays readable, but takes no more changes.
func Test_Cluster_Deleting_RefusesChanges(t *testing.T) {
	t.Parallel()
	q := newQuotaEnv(t)
	q.setNodeQuota(t, offeringN1Standard1, 4)
	clusterID, err := q.createCluster("c")
	require.NoError(t, err)
	poolID, err := q.createNodePool(clusterID, "only", "n1-standard-1", 2)
	require.NoError(t, err)

	_, err = q.client.DeleteCluster(q.ctx, organizationv1.DeleteClusterRequest_builder{ClusterId: clusterID}.Build())
	require.NoError(t, err)

	requireRefused(t, q.updateNodePool(poolID, 1, 1), "the cluster is being deleted")

	_, err = q.client.UpdateCluster(q.ctx, organizationv1.UpdateClusterRequest_builder{ClusterId: clusterID}.Build())
	requireRefused(t, err, "the cluster is being deleted")
	_, err = q.client.UpdateCluster(q.ctx, organizationv1.UpdateClusterRequest_builder{ClusterId: clusterID, KubernetesVersion: new("1.29")}.Build())
	requireRefused(t, err, "the cluster is being deleted")
	_, err = q.client.UpdateClusterDefaults(q.ctx, organizationv1.UpdateClusterDefaultsRequest_builder{ClusterId: clusterID}.Build())
	requireRefused(t, err, "the cluster is being deleted")

	projects := organizationv1connect.NewProjectServiceClient(q.env.server.Client(), q.env.server.URL)
	_, err = projects.CreateProject(q.ctx, organizationv1.CreateProjectRequest_builder{
		ClusterId: clusterID,
		Name:      "late",
	}.Build())
	requireRefused(t, err, "the cluster is being deleted")
}
