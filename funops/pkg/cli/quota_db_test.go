package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	db "github.com/fundament-oss/fundament/funops/pkg/db/gen"
)

// Quotas cap what an organization may build. The triggers that enforce them
// are exercised through the organization-api; here the commands that set and
// show them are, against the catalog fixtures from db/testdata (regions
// `local` and `eu-west-1`).

func quotaRows(t *testing.T, ctx *Context, organization string) (db.QuotaClustersGetRow, []db.MachineQuotaListRow) {
	t.Helper()

	orgID, err := lookupOrganizationID(t.Context(), ctx.Queries, organization)
	require.NoError(t, err)
	clusters, err := ctx.Queries.QuotaClustersGet(t.Context(), db.QuotaClustersGetParams{ID: orgID})
	require.NoError(t, err)
	machines, err := ctx.Queries.MachineQuotaList(t.Context(), db.MachineQuotaListParams{OrganizationID: orgID})
	require.NoError(t, err)
	return clusters, machines
}

func TestOrganizationQuota_NewOrganizationStartsAtOneClusterAndNoNodes(t *testing.T) {
	ctx := newTestContext(t)

	require.NoError(t, (&OrganizationCreateCmd{Name: "fresh"}).Run(ctx))

	clusters, machines := quotaRows(t, ctx, "fresh")
	assert.Equal(t, int32(1), clusters.QuotaClusters)
	assert.Equal(t, int64(0), clusters.ClustersInUse)
	assert.Empty(t, machines, "no machine quota until an operator sets one")

	require.NoError(t, (&OrganizationQuotaListCmd{Organization: "fresh"}).Run(ctx))
}

func TestOrganizationQuotaSetClusters(t *testing.T) {
	ctx := newTestContext(t)

	require.NoError(t, (&OrganizationQuotaSetClustersCmd{Organization: "globex", Clusters: 3}).Run(ctx))
	clusters, _ := quotaRows(t, ctx, "globex")
	assert.Equal(t, int32(3), clusters.QuotaClusters)

	// Zero is allowed: it stops every new cluster.
	require.NoError(t, (&OrganizationQuotaSetClustersCmd{Organization: "globex", Clusters: 0}).Run(ctx))
	clusters, _ = quotaRows(t, ctx, "globex")
	assert.Equal(t, int32(0), clusters.QuotaClusters)

	err := (&OrganizationQuotaSetClustersCmd{Organization: "globex", Clusters: -1}).Run(ctx)
	require.EqualError(t, err, "the cluster quota must be 0 or more")

	err = (&OrganizationQuotaSetClustersCmd{Organization: "nobody", Clusters: 1}).Run(ctx)
	require.EqualError(t, err, `organization "nobody" not found`)
}

func TestOrganizationQuotaSetNodes(t *testing.T) {
	ctx := newTestContext(t)

	require.NoError(t, (&OrganizationQuotaSetNodesCmd{Organization: "globex", Region: "local", MachineType: "local-small", MaxNodes: 10}).Run(ctx))

	_, machines := quotaRows(t, ctx, "globex")
	// The fixtures give globex 100 of every offering; local-small is now 10.
	var found *db.MachineQuotaListRow
	for i := range machines {
		if machines[i].Region == "local" && machines[i].MachineType == "local-small" {
			found = &machines[i]
		}
	}
	require.NotNil(t, found)
	assert.Equal(t, int32(10), found.MaxNodes)
	assert.Equal(t, int64(0), found.NodesInUse)
	firstUpdate := found.Updated.Time

	// Setting it again replaces the row rather than adding one.
	require.NoError(t, (&OrganizationQuotaSetNodesCmd{Organization: "globex", Region: "local", MachineType: "local-small", MaxNodes: 4}).Run(ctx))
	_, after := quotaRows(t, ctx, "globex")
	assert.Len(t, after, len(machines))
	for i := range after {
		if after[i].Region == "local" && after[i].MachineType == "local-small" {
			assert.Equal(t, int32(4), after[i].MaxNodes)
			assert.False(t, after[i].Updated.Time.Before(firstUpdate), "updated moves with the change")
		}
	}

	err := (&OrganizationQuotaSetNodesCmd{Organization: "globex", Region: "local", MachineType: "local-huge", MaxNodes: 1}).Run(ctx)
	require.EqualError(t, err, `machine type "local-huge" is not offered in region "local"`)

	err = (&OrganizationQuotaSetNodesCmd{Organization: "globex", Region: "local", MachineType: "local-small", MaxNodes: -1}).Run(ctx)
	require.EqualError(t, err, "the node quota must be 0 or more")

	require.NoError(t, (&OrganizationQuotaListCmd{Organization: "globex"}).Run(ctx))
}

func TestOrganizationQuotaList_ShowsWhatIsInUse(t *testing.T) {
	ctx := newTestContext(t)

	// acme-corp has one cluster in eu-west-1. Give it a pool of two
	// n1-standard-1 machines, with a maximum of 3.
	_, err := ctx.DB.Pool.Exec(t.Context(), `
		INSERT INTO tenant.node_pools (cluster_id, name, machine_type, autoscale_min, autoscale_max, region_machine_type_id)
		VALUES ('019b4000-2000-7000-8000-000000000001', 'workers', 'n1-standard-1', 2, 3, '019b4000-5300-7000-8000-000000000003')`)
	require.NoError(t, err)

	clusters, machines := quotaRows(t, ctx, "acme-corp")
	assert.Equal(t, int64(1), clusters.ClustersInUse)

	inUse := map[string]int64{}
	for i := range machines {
		inUse[machines[i].Region+"/"+machines[i].MachineType] = machines[i].NodesInUse
	}
	assert.Equal(t, int64(3), inUse["eu-west-1/n1-standard-1"], "the maximum counts, not the minimum")
	assert.Equal(t, int64(0), inUse["eu-west-1/n1-standard-2"])
}

func TestOrganizationQuotaSet_LoweringBelowUseIsAllowed(t *testing.T) {
	ctx := newTestContext(t)

	_, err := ctx.DB.Pool.Exec(t.Context(), `
		INSERT INTO tenant.node_pools (cluster_id, name, machine_type, autoscale_min, autoscale_max, region_machine_type_id)
		VALUES ('019b4000-2000-7000-8000-000000000001', 'workers', 'n1-standard-1', 1, 5, '019b4000-5300-7000-8000-000000000003')`)
	require.NoError(t, err)

	// Both quotas go below what acme-corp runs; nothing is touched, the list
	// just shows the excess.
	require.NoError(t, (&OrganizationQuotaSetClustersCmd{Organization: "acme-corp", Clusters: 0}).Run(ctx))
	require.NoError(t, (&OrganizationQuotaSetNodesCmd{Organization: "acme-corp", Region: "eu-west-1", MachineType: "n1-standard-1", MaxNodes: 2}).Run(ctx))

	clusters, machines := quotaRows(t, ctx, "acme-corp")
	assert.Equal(t, int32(0), clusters.QuotaClusters)
	assert.Equal(t, int64(1), clusters.ClustersInUse)
	for i := range machines {
		if machines[i].Region == "eu-west-1" && machines[i].MachineType == "n1-standard-1" {
			assert.Equal(t, int32(2), machines[i].MaxNodes)
			assert.Equal(t, int64(5), machines[i].NodesInUse)
		}
	}

	var maxNodes int
	err = ctx.DB.Pool.QueryRow(t.Context(),
		`SELECT autoscale_max FROM tenant.node_pools WHERE name = 'workers' AND deleted IS NULL`).Scan(&maxNodes)
	require.NoError(t, err)
	assert.Equal(t, 5, maxNodes, "the existing pool keeps its maximum")

	// A pool that grows is refused now; one that shrinks is not.
	_, err = ctx.DB.Pool.Exec(t.Context(), `UPDATE tenant.node_pools SET autoscale_max = 6 WHERE name = 'workers'`)
	require.ErrorContains(t, err, "would total 6 nodes, over the organization's quota of 2")
	_, err = ctx.DB.Pool.Exec(t.Context(), `UPDATE tenant.node_pools SET autoscale_max = 4 WHERE name = 'workers'`)
	require.NoError(t, err)
}
