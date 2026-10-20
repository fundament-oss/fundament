package organization_test

import (
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

const lastPoolMessage = "a cluster needs at least one node pool; add another node pool before removing this one"

func (q *quotaEnv) deleteNodePool(nodePoolID string) error {
	_, err := q.client.DeleteNodePool(q.ctx, organizationv1.DeleteNodePoolRequest_builder{NodePoolId: nodePoolID}.Build())
	return err
}

func requireLastPoolRefused(t *testing.T, err error) {
	t.Helper()
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
	assert.Equal(t, lastPoolMessage, connectErr.Message())
}

// A cluster runs only the pools it has, so its last pool stays.
func Test_NodePool_Delete_RefusesTheLastPool(t *testing.T) {
	t.Parallel()
	q := newQuotaEnv(t)
	q.setNodeQuota(t, offeringN1Standard1, 4)
	clusterID, err := q.createCluster("c")
	require.NoError(t, err)

	first, err := q.createNodePool(clusterID, "first", "n1-standard-1", 2)
	require.NoError(t, err)
	second, err := q.createNodePool(clusterID, "second", "n1-standard-1", 2)
	require.NoError(t, err)

	require.NoError(t, q.deleteNodePool(first))
	requireLastPoolRefused(t, q.deleteNodePool(second))
	assert.Equal(t, 1, q.livePools(t))

	// Replacing the last pool works the other way round: add, then remove.
	_, err = q.createNodePool(clusterID, "replacement", "n1-standard-1", 2)
	require.NoError(t, err)
	require.NoError(t, q.deleteNodePool(second))
	assert.Equal(t, 1, q.livePools(t))
}

// The console removes pools in parallel: two deletes of a cluster's last two
// pools must not both pass.
func Test_NodePool_Delete_ParallelDeletesKeepOnePool(t *testing.T) {
	t.Parallel()
	q := newQuotaEnv(t)
	q.setNodeQuota(t, offeringN1Standard1, 4)
	clusterID, err := q.createCluster("c")
	require.NoError(t, err)

	first, err := q.createNodePool(clusterID, "first", "n1-standard-1", 2)
	require.NoError(t, err)
	second, err := q.createNodePool(clusterID, "second", "n1-standard-1", 2)
	require.NoError(t, err)

	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i, poolID := range []string{first, second} {
		wg.Go(func() { errs[i] = q.deleteNodePool(poolID) })
	}
	wg.Wait()

	var refused int
	for _, err := range errs {
		if err != nil {
			requireLastPoolRefused(t, err)
			refused++
		}
	}
	assert.Equal(t, 1, refused, "exactly one delete wins")
	assert.Equal(t, 1, q.livePools(t))
}

// Deleting a cluster does not touch its pools; a pool of a deleted cluster may
// still be deleted, since the cluster is going away with its Shoot.
func Test_NodePool_Delete_PoolOfADeletedClusterMayGo(t *testing.T) {
	t.Parallel()
	q := newQuotaEnv(t)
	q.setNodeQuota(t, offeringN1Standard1, 4)
	clusterID, err := q.createCluster("c")
	require.NoError(t, err)
	poolID, err := q.createNodePool(clusterID, "only", "n1-standard-1", 2)
	require.NoError(t, err)

	_, err = q.client.DeleteCluster(q.ctx, organizationv1.DeleteClusterRequest_builder{ClusterId: clusterID}.Build())
	require.NoError(t, err)
	assert.Equal(t, 1, q.livePools(t), "deleting the cluster leaves its pools")

	require.NoError(t, q.deleteNodePool(poolID))
	assert.Zero(t, q.livePools(t))
}

func (q *quotaEnv) createSizedNodePool(clusterID, name string, minNodes, maxNodes int32) (string, error) {
	res, err := q.client.CreateNodePool(q.ctx, organizationv1.CreateNodePoolRequest_builder{
		ClusterId:    clusterID,
		Name:         name,
		MachineType:  "n1-standard-1",
		AutoscaleMin: minNodes,
		AutoscaleMax: maxNodes,
	}.Build())
	if err != nil {
		return "", err
	}
	return res.GetNodePoolId(), nil
}

func requireRefused(t *testing.T, err error, message string) {
	t.Helper()
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
	assert.Equal(t, message, connectErr.Message())
}

// Gardener refuses a Shoot whose every worker pool has a maximum of 0, so a
// cluster always keeps a pool that can run nodes.
func Test_NodePool_KeepsAPoolThatCanRunNodes(t *testing.T) {
	t.Parallel()
	q := newQuotaEnv(t)
	q.setNodeQuota(t, offeringN1Standard1, 4)
	clusterID, err := q.createCluster("c")
	require.NoError(t, err)

	_, err = q.createSizedNodePool(clusterID, "empty", 0, 0)
	requireRefused(t, err, "a cluster's first node pool needs a maximum above 0")
	assert.Zero(t, q.livePools(t))

	workers, err := q.createSizedNodePool(clusterID, "workers", 1, 2)
	require.NoError(t, err)
	spare, err := q.createSizedNodePool(clusterID, "spare", 0, 0)
	require.NoError(t, err, "an empty pool next to one that can run nodes is fine")

	requireRefused(t, q.updateNodePool(workers, 0, 0), "a cluster needs at least one node pool with a maximum above 0")
	requireRefused(t, q.deleteNodePool(workers), "a cluster needs at least one node pool with a maximum above 0")

	// Once the spare can run nodes, workers may shrink to 0 or go.
	require.NoError(t, q.updateNodePool(spare, 0, 1))
	require.NoError(t, q.updateNodePool(workers, 0, 0))
	require.NoError(t, q.deleteNodePool(workers))
	assert.Equal(t, 1, q.livePools(t))
}
