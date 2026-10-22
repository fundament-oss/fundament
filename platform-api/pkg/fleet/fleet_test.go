package fleet_test

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/platform-api/internal/testpg"
	fleetv1 "github.com/fundament-oss/fundament/platform-api/pkg/proto/gen/fleet/v1"
)

func clusterIDs(clusters []*fleetv1.ClusterSummary) []string {
	ids := make([]string, 0, len(clusters))
	for _, c := range clusters {
		ids = append(ids, c.GetId())
	}
	return ids
}

// Soft-deleted organizations, clusters, node pools, projects and namespaces
// are left out of every list and count.
func TestFleetExcludesDeletedRows(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	seed := env.Seed(t)
	client := staffClient(t, env)

	org := seed.Organization("fleet-org", "Fleet Org")
	live := seed.Cluster(org, "live")
	seed.Ready(live, time.Minute)
	seed.ClusterSnapshot(live, time.Minute, 3, 3, 40, 330)
	seed.NodePool(live, "workers", 1, 3)
	seed.SoftDelete("node_pools", seed.NodePool(live, "retired", 1, 3))
	project := seed.Project(live, "project")
	seed.Namespace(project, "ns-live")
	seed.SoftDelete("namespaces", seed.Namespace(project, "ns-deleted"))
	deletedProject := seed.Project(live, "deleted-project")
	seed.SoftDelete("projects", deletedProject)

	deleted := seed.Cluster(org, "deleted")
	seed.ClusterSnapshot(deleted, time.Minute, 5, 5, 50, 550)
	seed.SoftDelete("clusters", deleted)

	gone := seed.Organization("fleet-gone", "Gone")
	goneCluster := seed.Cluster(gone, "of-gone-org")
	seed.SoftDelete("organizations", gone)

	orgs, err := client.ListOrganizations(t.Context(), &fleetv1.ListOrganizationsRequest{})
	require.NoError(t, err)
	var found *fleetv1.OrganizationSummary
	for _, o := range orgs.GetOrganizations() {
		assert.NotEqual(t, "fleet-gone", o.GetName())
		assert.NotEqual(t, "system", o.GetName(), "the seeded system organization is no tenant")
		if o.GetName() == "fleet-org" {
			found = o
		}
	}
	require.NotNil(t, found)
	assert.Equal(t, int32(1), found.GetClusters())
	assert.Equal(t, int32(3), found.GetNodes())
	assert.Equal(t, int32(40), found.GetPods())
	assert.Equal(t, int32(1), found.GetNamespaces())

	clusters, err := client.ListClusters(t.Context(), fleetv1.ListClustersRequest_builder{
		OrganizationId: new(org.String()),
	}.Build())
	require.NoError(t, err)
	assert.Equal(t, []string{live.String()}, clusterIDs(clusters.GetClusters()))

	all, err := client.ListClusters(t.Context(), &fleetv1.ListClustersRequest{})
	require.NoError(t, err)
	assert.NotContains(t, clusterIDs(all.GetClusters()), goneCluster.String(), "a cluster of a deleted organization is not live")

	_, err = client.GetCluster(t.Context(), fleetv1.GetClusterRequest_builder{ClusterId: deleted.String()}.Build())
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))

	_, err = client.GetOrganization(t.Context(), fleetv1.GetOrganizationRequest_builder{OrganizationId: gone.String()}.Build())
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))

	detail, err := client.GetCluster(t.Context(), fleetv1.GetClusterRequest_builder{ClusterId: live.String()}.Build())
	require.NoError(t, err)
	require.Len(t, detail.GetNodePools(), 1)
	assert.Equal(t, "workers", detail.GetNodePools()[0].GetName())
	require.Len(t, detail.GetProjects(), 1)
	require.Len(t, detail.GetProjects()[0].GetNamespaces(), 1)
	assert.Equal(t, "ns-live", detail.GetProjects()[0].GetNamespaces()[0].GetName())
}

func TestFleetSummaryCountsSeededClusters(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	seed := env.Seed(t)
	client := staffClient(t, env)

	before, err := client.GetFleetSummary(t.Context(), &fleetv1.GetFleetSummaryRequest{})
	require.NoError(t, err)

	org := seed.Organization("summary-org", "Summary Org")
	healthy := seed.Cluster(org, "healthy")
	seed.Ready(healthy, time.Minute)
	seed.ClusterSnapshot(healthy, time.Minute, 4, 3, 60, 440)
	sick := seed.Cluster(org, "sick")
	seed.Ready(sick, time.Minute)
	seed.Status(sick, "ready", "unhealthy", false)
	seed.ClusterSnapshot(sick, time.Minute, 2, 1, 10, 220)
	updating := seed.Cluster(org, "updating")
	seed.Ready(updating, time.Minute)
	seed.Status(updating, "ready", "healthy", true)
	seed.ClusterSnapshot(updating, time.Minute, 1, 1, 5, 110)

	after, err := client.GetFleetSummary(t.Context(), &fleetv1.GetFleetSummaryRequest{})
	require.NoError(t, err)

	b, a := before.GetSummary(), after.GetSummary()
	assert.Equal(t, b.GetOrganizations()+1, a.GetOrganizations())
	assert.Equal(t, b.GetClusters()+3, a.GetClusters())
	assert.Equal(t, b.GetClustersUnhealthy()+1, a.GetClustersUnhealthy())
	assert.Equal(t, b.GetClustersUpdating()+1, a.GetClustersUpdating())
	assert.Equal(t, b.GetNodes()+7, a.GetNodes())
	assert.Equal(t, b.GetNodesReady()+5, a.GetNodesReady())
	assert.Equal(t, b.GetPods()+75, a.GetPods())
	assert.Equal(t, b.GetPodAllocatable()+770, a.GetPodAllocatable())
	assert.Equal(t, b.GetCpuAllocatableM()+7*4000, a.GetCpuAllocatableM())
	assert.Equal(t, int32(testStaleAfter.Seconds()), a.GetStaleAfterSeconds())
}

func TestListClustersFilters(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	seed := env.Seed(t)
	client := staffClient(t, env)

	org := seed.Organization("filter-org", "Filter Org")

	fine := seed.Cluster(org, "fine")
	seed.Ready(fine, time.Minute)
	seed.ClusterSnapshot(fine, time.Minute, 2, 2, 10, 220)
	seed.NodePool(fine, "workers", 1, 3)

	broken := seed.Cluster(org, "broken")
	seed.Status(broken, "error", "", false)

	stale := seed.Cluster(org, "stale")
	seed.Ready(stale, 2*time.Hour)
	seed.ClusterSnapshot(stale, 2*time.Hour, 2, 2, 10, 220)

	list := func(filter fleetv1.ClusterFilter) []string {
		t.Helper()
		resp, err := client.ListClusters(t.Context(), fleetv1.ListClustersRequest_builder{
			OrganizationId: new(org.String()),
			Filter:         filter,
		}.Build())
		require.NoError(t, err)
		return clusterIDs(resp.GetClusters())
	}

	assert.ElementsMatch(t, []string{fine.String(), broken.String(), stale.String()}, list(fleetv1.ClusterFilter_CLUSTER_FILTER_ALL))
	assert.ElementsMatch(t, []string{broken.String()}, list(fleetv1.ClusterFilter_CLUSTER_FILTER_UNHEALTHY))
	assert.ElementsMatch(t, []string{stale.String()}, list(fleetv1.ClusterFilter_CLUSTER_FILTER_STALE))
}

func TestGetClusterDetail(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	seed := env.Seed(t)
	client := staffClient(t, env)

	org := seed.Organization("detail-org", "Detail Org")
	cluster := seed.Cluster(org, "detail")
	seed.Ready(cluster, time.Minute)
	seed.ClusterSnapshot(cluster, time.Minute, 3, 3, 12, 330)
	pool := seed.NodePool(cluster, "workers", 2, 4)
	seed.NodePoolSnapshot(pool, 3, 3)
	unobserved := seed.NodePool(cluster, "fresh", 1, 2)
	project := seed.Project(cluster, "shop")
	namespace := seed.Namespace(project, "shop-prod")
	seed.NamespaceSnapshot(namespace, 7)
	seed.GardenerShoot(cluster, "detail-shoot", "1.31.1", time.Minute)

	resp, err := client.GetCluster(t.Context(), fleetv1.GetClusterRequest_builder{ClusterId: cluster.String()}.Build())
	require.NoError(t, err)

	c := resp.GetCluster()
	assert.Equal(t, "detail", c.GetName())
	assert.Equal(t, "detail-org", c.GetOrganizationName())
	assert.Equal(t, fleetv1.ClusterStatus_CLUSTER_STATUS_RUNNING, c.GetStatus())
	assert.Equal(t, fleetv1.ClusterHealth_CLUSTER_HEALTH_HEALTHY, c.GetHealth())
	assert.Equal(t, int32(3), c.GetNodesMin())
	assert.Equal(t, int32(6), c.GetNodesMax())
	assert.Equal(t, int32(3), c.GetSnapshot().GetNodes())
	assert.True(t, c.GetSnapshot().HasObserved())
	assert.False(t, c.GetSnapshot().HasLastError())
	assert.False(t, c.GetStale())

	require.True(t, resp.HasShoot())
	assert.Equal(t, "detail-shoot", resp.GetShoot().GetName())

	pools := map[string]*fleetv1.NodePool{}
	for _, p := range resp.GetNodePools() {
		pools[p.GetId()] = p
	}
	require.Contains(t, pools, pool.String())
	require.Contains(t, pools, unobserved.String())
	assert.Equal(t, int32(3), pools[pool.String()].GetNodes())
	assert.True(t, pools[pool.String()].HasObserved())
	assert.False(t, pools[unobserved.String()].HasObserved(), "a pool without a snapshot has no observed time")

	require.Len(t, resp.GetProjects(), 1)
	ns := resp.GetProjects()[0].GetNamespaces()
	require.Len(t, ns, 1)
	assert.Equal(t, int32(7), ns[0].GetPods())
}

func TestGetClusterNotFound(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	_, err := staffClient(t, env).GetCluster(t.Context(), fleetv1.GetClusterRequest_builder{ClusterId: uuid.NewString()}.Build())
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}
