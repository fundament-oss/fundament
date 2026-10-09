package organization_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// Catalog fixtures from db/testdata: region eu-west-1 offers n1-standard-1 and
// n1-standard-2.
const (
	offeringN1Standard1 = "019b4000-5300-7000-8000-000000000003"
	offeringN1Standard2 = "019b4000-5300-7000-8000-000000000004"
)

func (q *quotaEnv) setNodeQuota(t *testing.T, offering string, maxNodes int) {
	t.Helper()
	_, err := q.env.adminPool.Exec(t.Context(), `
		INSERT INTO tenant.organization_machine_quotas (organization_id, region_machine_type_id, max_nodes)
		VALUES ($1, $2, $3)
		ON CONFLICT (organization_id, region_machine_type_id) DO UPDATE SET max_nodes = EXCLUDED.max_nodes`,
		q.orgID, offering, maxNodes)
	require.NoError(t, err)
}

func (q *quotaEnv) createNodePool(clusterID, name, machineType string, maxNodes int32) (string, error) {
	res, err := q.client.CreateNodePool(q.ctx, organizationv1.CreateNodePoolRequest_builder{
		ClusterId:    clusterID,
		Name:         name,
		MachineType:  machineType,
		AutoscaleMin: 1,
		AutoscaleMax: maxNodes,
	}.Build())
	if err != nil {
		return "", err
	}
	return res.GetNodePoolId(), nil
}

func (q *quotaEnv) updateNodePool(nodePoolID string, minNodes, maxNodes int32) error {
	_, err := q.client.UpdateNodePool(q.ctx, organizationv1.UpdateNodePoolRequest_builder{
		NodePoolId:   nodePoolID,
		AutoscaleMin: minNodes,
		AutoscaleMax: maxNodes,
	}.Build())
	return err
}

func (q *quotaEnv) livePools(t *testing.T) int {
	t.Helper()
	var n int
	err := q.env.adminPool.QueryRow(t.Context(), `
		SELECT count(*)
		FROM tenant.node_pools
		JOIN tenant.clusters ON tenant.clusters.id = tenant.node_pools.cluster_id
		WHERE tenant.clusters.organization_id = $1 AND tenant.node_pools.deleted IS NULL`, q.orgID).Scan(&n)
	require.NoError(t, err)
	return n
}

func Test_NodePool_Create_NoQuotaRefusesEverything(t *testing.T) {
	t.Parallel()
	q := newQuotaEnv(t)
	clusterID, err := q.createCluster("c")
	require.NoError(t, err)

	_, err = q.createNodePool(clusterID, "workers", "n1-standard-1", 1)
	requireResourceExhausted(t, err, "the organization has no quota for machine type n1-standard-1 in region eu-west-1")
	assert.Zero(t, q.livePools(t))
}

func Test_NodePool_Create_SumOfMaximaStaysWithinQuota(t *testing.T) {
	t.Parallel()
	q := newQuotaEnv(t)
	clusterID, err := q.createCluster("c")
	require.NoError(t, err)
	q.setNodeQuota(t, offeringN1Standard1, 5)

	_, err = q.createNodePool(clusterID, "a", "n1-standard-1", 3)
	require.NoError(t, err)

	// 3 + 3 > 5: refused, and the message says the sum and the quota.
	_, err = q.createNodePool(clusterID, "b", "n1-standard-1", 3)
	requireResourceExhausted(t, err, "node pools of machine type n1-standard-1 in region eu-west-1 would total 6 nodes, over the organization's quota of 5")
	assert.Equal(t, 1, q.livePools(t))

	// 3 + 2 = 5: the quota is a maximum, not a bound to stay under.
	_, err = q.createNodePool(clusterID, "b", "n1-standard-1", 2)
	require.NoError(t, err)

	// Another machine type has its own quota, and this organization has none
	// for it.
	_, err = q.createNodePool(clusterID, "big", "n1-standard-2", 1)
	requireResourceExhausted(t, err, "the organization has no quota for machine type n1-standard-2 in region eu-west-1")
	q.setNodeQuota(t, offeringN1Standard2, 1)
	_, err = q.createNodePool(clusterID, "big", "n1-standard-2", 1)
	require.NoError(t, err)
}

// The quota is per organization: pools on every cluster of it add up.
func Test_NodePool_Create_QuotaSpansClusters(t *testing.T) {
	t.Parallel()
	q := newQuotaEnv(t)
	q.setClusterQuota(t, 2)
	q.setNodeQuota(t, offeringN1Standard1, 4)

	first, err := q.createCluster("first")
	require.NoError(t, err)
	second, err := q.createCluster("second")
	require.NoError(t, err)

	_, err = q.createNodePool(first, "workers", "n1-standard-1", 3)
	require.NoError(t, err)
	_, err = q.createNodePool(second, "workers", "n1-standard-1", 2)
	requireResourceExhausted(t, err, "node pools of machine type n1-standard-1 in region eu-west-1 would total 5 nodes, over the organization's quota of 4")
	_, err = q.createNodePool(second, "workers", "n1-standard-1", 1)
	require.NoError(t, err)
}

func Test_NodePool_Delete_FreesQuota(t *testing.T) {
	t.Parallel()
	q := newQuotaEnv(t)
	clusterID, err := q.createCluster("c")
	require.NoError(t, err)
	q.setNodeQuota(t, offeringN1Standard1, 3)

	poolID, err := q.createNodePool(clusterID, "a", "n1-standard-1", 3)
	require.NoError(t, err)
	_, err = q.createNodePool(clusterID, "b", "n1-standard-1", 1)
	requireResourceExhausted(t, err, "node pools of machine type n1-standard-1 in region eu-west-1 would total 4 nodes, over the organization's quota of 3")

	_, err = q.client.DeleteNodePool(q.ctx, organizationv1.DeleteNodePoolRequest_builder{NodePoolId: poolID}.Build())
	require.NoError(t, err)

	_, err = q.createNodePool(clusterID, "b", "n1-standard-1", 3)
	require.NoError(t, err)
}

// A deleted cluster's pools keep counting until Gardener has torn the shoot
// down, as the cluster itself does against the cluster quota.
func Test_NodePool_Create_DeletedClusterCountsUntilTornDown(t *testing.T) {
	t.Parallel()
	q := newQuotaEnv(t)
	q.setClusterQuota(t, 2)
	q.setNodeQuota(t, offeringN1Standard1, 3)

	first, err := q.createCluster("first")
	require.NoError(t, err)
	_, err = q.createNodePool(first, "workers", "n1-standard-1", 3)
	require.NoError(t, err)

	_, err = q.client.DeleteCluster(q.ctx, organizationv1.DeleteClusterRequest_builder{ClusterId: first}.Build())
	require.NoError(t, err)

	second, err := q.createCluster("second")
	require.NoError(t, err)
	_, err = q.createNodePool(second, "workers", "n1-standard-1", 1)
	requireResourceExhausted(t, err, "node pools of machine type n1-standard-1 in region eu-west-1 would total 4 nodes, over the organization's quota of 3")

	_, err = q.env.adminPool.Exec(t.Context(), `UPDATE tenant.clusters SET shoot_status = 'deleted' WHERE id = $1`, first)
	require.NoError(t, err)
	_, err = q.createNodePool(second, "workers", "n1-standard-1", 3)
	require.NoError(t, err)
}

// Lowering a quota under what the organization runs leaves its pools alone:
// they may still shrink, change their minimum, or stay; only growth is refused.
func Test_NodePool_Update_LoweredQuotaRefusesOnlyGrowth(t *testing.T) {
	t.Parallel()
	q := newQuotaEnv(t)
	clusterID, err := q.createCluster("c")
	require.NoError(t, err)
	q.setNodeQuota(t, offeringN1Standard1, 5)

	poolID, err := q.createNodePool(clusterID, "a", "n1-standard-1", 5)
	require.NoError(t, err)

	q.setNodeQuota(t, offeringN1Standard1, 2)

	require.NoError(t, q.updateNodePool(poolID, 2, 5), "the same maximum is not growth")
	require.NoError(t, q.updateNodePool(poolID, 1, 4), "shrinking is allowed even while over the quota")

	err = q.updateNodePool(poolID, 1, 6)
	requireResourceExhausted(t, err, "node pools of machine type n1-standard-1 in region eu-west-1 would total 6 nodes, over the organization's quota of 2")

	var maxNodes int
	err = q.env.adminPool.QueryRow(t.Context(), `SELECT autoscale_max FROM tenant.node_pools WHERE id = $1`, poolID).Scan(&maxNodes)
	require.NoError(t, err)
	assert.Equal(t, 4, maxNodes)

	// Within the quota, growth is fine.
	q.setNodeQuota(t, offeringN1Standard1, 6)
	require.NoError(t, q.updateNodePool(poolID, 1, 6))
}
