package organization_test

import (
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func spec(name, machineType string, minNodes, maxNodes int32) *organizationv1.NodePoolSpec {
	return organizationv1.NodePoolSpec_builder{
		Name:         name,
		MachineType:  machineType,
		AutoscaleMin: minNodes,
		AutoscaleMax: maxNodes,
	}.Build()
}

func (q *quotaEnv) setNodePools(clusterID string, pools ...*organizationv1.NodePoolSpec) error {
	_, err := q.client.SetNodePools(q.ctx, organizationv1.SetNodePoolsRequest_builder{
		ClusterId: clusterID,
		NodePools: pools,
	}.Build())
	return err
}

// livePoolsByName returns name → id of the organization's live pools.
func (q *quotaEnv) livePoolsByName(t *testing.T, clusterID string) map[string]string {
	t.Helper()
	res, err := q.client.ListNodePools(q.ctx, organizationv1.ListNodePoolsRequest_builder{ClusterId: clusterID}.Build())
	require.NoError(t, err)
	pools := map[string]string{}
	for _, pool := range res.GetNodePools() {
		pools[pool.GetName()] = pool.GetId()
	}
	return pools
}

func requireConnectCode(t *testing.T, err error, code connect.Code) {
	t.Helper()
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, code, connectErr.Code(), connectErr.Message())
}

func Test_NodePool_Set_CreatesResizesAndRemoves(t *testing.T) {
	t.Parallel()
	q := newQuotaEnv(t)
	q.setNodeQuota(t, offeringN1Standard1, 10)
	clusterID, err := q.createCluster("c")
	require.NoError(t, err)

	require.NoError(t, q.setNodePools(clusterID, spec("a", "n1-standard-1", 1, 2), spec("b", "n1-standard-1", 1, 2)))
	before := q.livePoolsByName(t, clusterID)
	require.Len(t, before, 2)

	// a keeps its identity across a resize, b goes, c is new.
	require.NoError(t, q.setNodePools(clusterID, spec("a", "n1-standard-1", 2, 4), spec("c", "n1-standard-1", 1, 1)))
	after := q.livePoolsByName(t, clusterID)
	assert.ElementsMatch(t, []string{"a", "c"}, keys(after))
	assert.Equal(t, before["a"], after["a"], "a resize is an update, not a replacement")

	res, err := q.client.ListNodePools(q.ctx, organizationv1.ListNodePoolsRequest_builder{ClusterId: clusterID}.Build())
	require.NoError(t, err)
	sizes := map[string][3]any{}
	for _, pool := range res.GetNodePools() {
		sizes[pool.GetName()] = [3]any{pool.GetMachineType(), pool.GetMinNodes(), pool.GetMaxNodes()}
	}
	assert.Equal(t, [3]any{"n1-standard-1", int32(2), int32(4)}, sizes["a"], "a was resized")
	assert.Equal(t, [3]any{"n1-standard-1", int32(1), int32(1)}, sizes["c"])
}

// A form built from an earlier read must not remove a pool someone else added
// since: with expected_current set, a changed cluster refuses the set.
func Test_NodePool_Set_RefusesAStaleExpectedSet(t *testing.T) {
	t.Parallel()
	q := newQuotaEnv(t)
	q.setNodeQuota(t, offeringN1Standard1, 10)
	clusterID, err := q.createCluster("c")
	require.NoError(t, err)

	setExpecting := func(expected []string, pools ...*organizationv1.NodePoolSpec) error {
		_, err := q.client.SetNodePools(q.ctx, organizationv1.SetNodePoolsRequest_builder{
			ClusterId:       clusterID,
			NodePools:       pools,
			ExpectedCurrent: organizationv1.ExpectedNodePools_builder{NodePoolIds: expected}.Build(),
		}.Build())
		return err
	}

	// A new cluster: the wizard expects no pools.
	require.NoError(t, setExpecting(nil, spec("a", "n1-standard-1", 1, 1)))
	read := q.livePoolsByName(t, clusterID)

	// Someone else adds a pool after the form was read.
	_, err = q.createNodePool(clusterID, "theirs", "n1-standard-1", 1)
	require.NoError(t, err)

	err = setExpecting([]string{read["a"]}, spec("a", "n1-standard-1", 1, 2))
	requireConnectCode(t, err, connect.CodeAborted)
	assert.ElementsMatch(t, []string{"a", "theirs"}, keys(q.livePoolsByName(t, clusterID)), "nothing removed")

	fresh := q.livePoolsByName(t, clusterID)
	require.NoError(t, setExpecting([]string{fresh["a"], fresh["theirs"]}, spec("a", "n1-standard-1", 1, 2)))
	assert.ElementsMatch(t, []string{"a"}, keys(q.livePoolsByName(t, clusterID)))
}

// The case the separate calls could not do: the only pool, renamed, while it
// already uses the whole quota. Removal and addition happen at once.
func Test_NodePool_Set_RenamesTheOnlyPoolAtFullQuota(t *testing.T) {
	t.Parallel()
	q := newQuotaEnv(t)
	q.setNodeQuota(t, offeringN1Standard1, 3)
	clusterID, err := q.createCluster("c")
	require.NoError(t, err)
	require.NoError(t, q.setNodePools(clusterID, spec("workers", "n1-standard-1", 1, 3)))

	require.NoError(t, q.setNodePools(clusterID, spec("general", "n1-standard-1", 1, 3)))
	assert.ElementsMatch(t, []string{"general"}, keys(q.livePoolsByName(t, clusterID)))
}

func Test_NodePool_Set_ReplacesAPoolWhoseMachineTypeChanges(t *testing.T) {
	t.Parallel()
	q := newQuotaEnv(t)
	q.setNodeQuota(t, offeringN1Standard1, 3)
	q.setNodeQuota(t, offeringN1Standard2, 3)
	clusterID, err := q.createCluster("c")
	require.NoError(t, err)
	require.NoError(t, q.setNodePools(clusterID, spec("workers", "n1-standard-1", 1, 3)))
	before := q.livePoolsByName(t, clusterID)

	require.NoError(t, q.setNodePools(clusterID, spec("workers", "n1-standard-2", 1, 3)))
	after := q.livePoolsByName(t, clusterID)
	require.Len(t, after, 1)
	assert.NotEqual(t, before["workers"], after["workers"], "a new pool under the same name")
}

// A pool from before the catalog, of a type the region does not offer, keeps
// running: a set that leaves it as it is changes the other pools.
func Test_NodePool_Set_KeepsAPoolFromBeforeTheCatalog(t *testing.T) {
	t.Parallel()
	q := newQuotaEnv(t)
	q.setNodeQuota(t, offeringN1Standard1, 10)
	clusterID, err := q.createCluster("c")
	require.NoError(t, err)
	require.NoError(t, q.setNodePools(clusterID, spec("workers", "n1-standard-1", 1, 2)))

	for _, statement := range []string{
		`ALTER TABLE tenant.node_pools DISABLE TRIGGER verify_quota`,
		`INSERT INTO tenant.node_pools (cluster_id, name, machine_type, autoscale_min, autoscale_max)
		 VALUES ($1, 'old', 'retired-type', 1, 1)`,
		`ALTER TABLE tenant.node_pools ENABLE TRIGGER verify_quota`,
	} {
		if strings.Contains(statement, "$1") {
			_, err = q.env.adminPool.Exec(t.Context(), statement, clusterID)
		} else {
			_, err = q.env.adminPool.Exec(t.Context(), statement)
		}
		require.NoError(t, err)
	}
	before := q.livePoolsByName(t, clusterID)

	require.NoError(t, q.setNodePools(clusterID, spec("workers", "n1-standard-1", 1, 3), spec("old", "retired-type", 1, 1)))
	assert.Equal(t, before, q.livePoolsByName(t, clusterID), "both keep their identity")

	// Adding a pool of that type is still refused.
	err = q.setNodePools(clusterID, spec("workers", "n1-standard-1", 1, 3), spec("old", "retired-type", 1, 1), spec("new", "retired-type", 1, 1))
	requireConnectCode(t, err, connect.CodeInvalidArgument)
}

// A refused set changes nothing: no half-applied state.
func Test_NodePool_Set_IsAllOrNothing(t *testing.T) {
	t.Parallel()
	q := newQuotaEnv(t)
	q.setNodeQuota(t, offeringN1Standard1, 3)
	clusterID, err := q.createCluster("c")
	require.NoError(t, err)
	require.NoError(t, q.setNodePools(clusterID, spec("workers", "n1-standard-1", 1, 3)))
	before := q.livePoolsByName(t, clusterID)

	err = q.setNodePools(clusterID, spec("workers", "n1-standard-1", 1, 3), spec("more", "n1-standard-1", 1, 1))
	requireResourceExhausted(t, err, "node pools of machine type n1-standard-1 in region eu-west-1 would total 4 nodes, over the organization's quota of 3")
	assert.Equal(t, before, q.livePoolsByName(t, clusterID))

	err = q.setNodePools(clusterID, spec("other", "n1-highmem-99", 1, 1))
	requireConnectCode(t, err, connect.CodeInvalidArgument)
	assert.Equal(t, before, q.livePoolsByName(t, clusterID))
}

func Test_NodePool_Set_RefusesSetsWithoutAPoolThatCanRunNodes(t *testing.T) {
	t.Parallel()
	q := newQuotaEnv(t)
	q.setNodeQuota(t, offeringN1Standard1, 3)
	clusterID, err := q.createCluster("c")
	require.NoError(t, err)

	requireRefused(t, q.setNodePools(clusterID, spec("empty", "n1-standard-1", 0, 0)), "a cluster needs at least one node pool with a maximum above 0")
	requireConnectCode(t, q.setNodePools(clusterID), connect.CodeInvalidArgument)
	requireConnectCode(t, q.setNodePools(clusterID, spec("a", "n1-standard-1", 1, 1), spec("a", "n1-standard-1", 1, 1)), connect.CodeInvalidArgument)
	assert.Zero(t, q.livePools(t))
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
