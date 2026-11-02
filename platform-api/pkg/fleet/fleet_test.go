package fleet_test

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commongardener "github.com/fundament-oss/fundament/common/gardener"
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
	// Awaiting its first health checks: starting, as the console says, not
	// unhealthy.
	starting := seed.Cluster(org, "starting")
	seed.Ready(starting, time.Minute)
	seed.Status(starting, "ready", "unhealthy", false)
	_, err = env.Super.Exec(t.Context(), `UPDATE tenant.clusters SET shoot_status_message = $2 WHERE id = $1`,
		starting, commongardener.MsgShootAwaitingHealth)
	require.NoError(t, err)
	// The inventory only collects ready clusters, so the last numbers of one
	// in error are no longer counted.
	failed := seed.Cluster(org, "failed")
	seed.ClusterSnapshot(failed, time.Hour, 9, 9, 90, 990)
	seed.Status(failed, "error", "", false)
	seed.Heartbeat("inventory", 15*time.Minute, time.Minute)
	seed.Heartbeat("reconcile", 5*time.Minute, time.Minute)

	after, err := client.GetFleetSummary(t.Context(), &fleetv1.GetFleetSummaryRequest{})
	require.NoError(t, err)

	b, a := before.GetSummary(), after.GetSummary()
	assert.Equal(t, b.GetOrganizations()+1, a.GetOrganizations())
	assert.Equal(t, b.GetClusters()+5, a.GetClusters())
	assert.Equal(t, b.GetClustersUnhealthy()+2, a.GetClustersUnhealthy(), "the sick one and the one in error")
	assert.Equal(t, b.GetClustersUpdating()+1, a.GetClustersUpdating())
	assert.Equal(t, b.GetNodes()+7, a.GetNodes())
	assert.Equal(t, b.GetNodesReady()+5, a.GetNodesReady())
	assert.Equal(t, b.GetPods()+75, a.GetPods())
	assert.Equal(t, b.GetPodAllocatable()+770, a.GetPodAllocatable())
	assert.Equal(t, b.GetCpuAllocatableM()+7*4000, a.GetCpuAllocatableM())
	assert.Equal(t, int32((45 * time.Minute).Seconds()), a.GetSnapshotStaleAfterSeconds(), "three inventory intervals")
	require.True(t, a.HasInventory())
	assert.Equal(t, int32(900), a.GetInventory().GetIntervalSeconds())
	require.True(t, a.HasReconcile())
	assert.Equal(t, int32(300), a.GetReconcile().GetIntervalSeconds())
	assert.False(t, b.HasReconcile(), "absent before the first pass")
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

	lost := seed.Cluster(org, "lost")
	seed.Status(lost, "lost", "", false)

	seed.Heartbeat("inventory", 15*time.Minute, time.Minute)

	list := func(filter fleetv1.ClusterFilter) []string {
		t.Helper()
		resp, err := client.ListClusters(t.Context(), fleetv1.ListClustersRequest_builder{
			OrganizationId: new(org.String()),
			Filter:         filter,
		}.Build())
		require.NoError(t, err)
		return clusterIDs(resp.GetClusters())
	}

	assert.ElementsMatch(t, []string{fine.String(), broken.String(), stale.String(), lost.String()}, list(fleetv1.ClusterFilter_CLUSTER_FILTER_ALL))
	assert.ElementsMatch(t, []string{broken.String(), lost.String()}, list(fleetv1.ClusterFilter_CLUSTER_FILTER_UNHEALTHY))
	assert.ElementsMatch(t, []string{stale.String()}, list(fleetv1.ClusterFilter_CLUSTER_FILTER_STALE))
}

// Staleness is judged by the interval the inventory loop recorded, so a
// deployment collecting every minute and one collecting every hour each get
// their own cutoff. Before the loop ever ran nothing is called stale.
func TestStaleFollowsInventoryInterval(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	seed := env.Seed(t)
	client := staffClient(t, env)

	org := seed.Organization("interval-org", "Interval Org")
	cluster := seed.Cluster(org, "twenty-minutes")
	seed.Ready(cluster, 2*time.Hour)
	seed.ClusterSnapshot(cluster, 20*time.Minute, 1, 1, 1, 110)

	isStale := func() bool {
		t.Helper()
		resp, err := client.GetCluster(t.Context(), fleetv1.GetClusterRequest_builder{ClusterId: cluster.String()}.Build())
		require.NoError(t, err)
		return resp.GetCluster().GetStale()
	}

	assert.False(t, isStale(), "no inventory pass recorded yet")
	seed.Heartbeat("inventory", time.Minute, 0)
	assert.True(t, isStale(), "20 minutes is more than three 1-minute intervals")
	seed.Heartbeat("inventory", 15*time.Minute, 0)
	assert.False(t, isStale(), "20 minutes is less than three 15-minute intervals")
}

func TestListClustersPages(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	seed := env.Seed(t)
	client := staffClient(t, env)

	org := seed.Organization("paged-org", "Paged Org")
	var want []string
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		want = append(want, seed.Cluster(org, name).String())
	}
	// Two of five in error, so a filtered page has to read past the first
	// batch to fill up.
	for _, id := range want[2:4] {
		seed.Status(uuid.MustParse(id), "error", "", false)
	}

	collect := func(filter fleetv1.ClusterFilter, size int32) ([]string, int) {
		t.Helper()
		var ids []string
		token := ""
		pages := 0
		for {
			resp, err := client.ListClusters(t.Context(), fleetv1.ListClustersRequest_builder{
				OrganizationId: new(org.String()),
				Filter:         filter,
				PageSize:       size,
				PageToken:      token,
			}.Build())
			require.NoError(t, err)
			pages++
			assert.LessOrEqual(t, len(resp.GetClusters()), int(size))
			assert.NotEmpty(t, resp.GetClusters(), "no page is empty, the last one included")
			ids = append(ids, clusterIDs(resp.GetClusters())...)
			token = resp.GetNextPageToken()
			if token == "" || pages > 10 {
				return ids, pages
			}
		}
	}

	all, pages := collect(fleetv1.ClusterFilter_CLUSTER_FILTER_ALL, 2)
	assert.Equal(t, want, all, "every cluster once, by name")
	assert.Equal(t, 3, pages)

	broken, _ := collect(fleetv1.ClusterFilter_CLUSTER_FILTER_UNHEALTHY, 1)
	assert.Equal(t, want[2:4], broken)
}

func TestListOrganizationsPages(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	seed := env.Seed(t)
	client := staffClient(t, env)

	for _, name := range []string{"page-a", "page-b", "page-c"} {
		seed.Organization(name, name)
	}

	// The template holds organizations of its own, so walk every page.
	var names []string
	token := ""
	for pages := 0; ; pages++ {
		require.Less(t, pages, 20, "paging does not end")
		resp, err := client.ListOrganizations(t.Context(), fleetv1.ListOrganizationsRequest_builder{
			PageSize: 2, PageToken: token,
		}.Build())
		require.NoError(t, err)
		assert.LessOrEqual(t, len(resp.GetOrganizations()), 2)
		for _, o := range resp.GetOrganizations() {
			names = append(names, o.GetName())
		}
		token = resp.GetNextPageToken()
		if token == "" {
			break
		}
	}
	assert.Subset(t, names, []string{"page-a", "page-b", "page-c"})
	assert.NotContains(t, names, "system")
}

// The system organization is no tenant: its clusters are in no list, so the
// list agrees with the summary.
func TestFleetSkipsSystemOrganization(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	seed := env.Seed(t)
	client := staffClient(t, env)

	system := seed.Cluster(testpg.SystemOrganizationID, "plugins")

	all, err := client.ListClusters(t.Context(), &fleetv1.ListClustersRequest{})
	require.NoError(t, err)
	assert.NotContains(t, clusterIDs(all.GetClusters()), system.String())

	_, err = client.GetCluster(t.Context(), fleetv1.GetClusterRequest_builder{ClusterId: system.String()}.Build())
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))

	summary, err := client.GetFleetSummary(t.Context(), &fleetv1.GetFleetSummaryRequest{})
	require.NoError(t, err)
	assert.Equal(t, int32(len(all.GetClusters())), summary.GetSummary().GetClusters())
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
