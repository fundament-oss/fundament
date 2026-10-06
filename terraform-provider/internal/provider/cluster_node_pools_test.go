package provider

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

func pool(name, machineType string, autoscaleMin, autoscaleMax int64) ClusterNodePoolModel {
	return ClusterNodePoolModel{
		Name:         types.StringValue(name),
		MachineType:  types.StringValue(machineType),
		AutoscaleMin: types.Int64Value(autoscaleMin),
		AutoscaleMax: types.Int64Value(autoscaleMax),
	}
}

func TestPlanNodePoolChanges(t *testing.T) {
	current := []existingNodePool{
		{ID: "id-keep", Pool: pool("keep", "small", 1, 3)},
		{ID: "id-resize", Pool: pool("resize", "small", 1, 3)},
		{ID: "id-retype", Pool: pool("retype", "small", 1, 3)},
		{ID: "id-gone", Pool: pool("gone", "small", 1, 3)},
	}
	planned := []ClusterNodePoolModel{
		pool("keep", "small", 1, 3),
		pool("resize", "small", 2, 5),
		pool("retype", "large", 1, 3),
		pool("new", "small", 1, 1),
	}

	changes := planNodePoolChanges(current, planned)

	assert.Equal(t, []ClusterNodePoolModel{pool("retype", "large", 1, 3), pool("new", "small", 1, 1)}, changes.Create)
	assert.Equal(t, []existingNodePool{{ID: "id-resize", Pool: pool("resize", "small", 2, 5)}}, changes.Update)
	assert.Equal(t, []existingNodePool{
		{ID: "id-retype", Pool: pool("retype", "small", 1, 3)},
		{ID: "id-gone", Pool: pool("gone", "small", 1, 3)},
	}, changes.Delete)
}

func TestPlanNodePoolChangesNoChange(t *testing.T) {
	current := []existingNodePool{{ID: "id-a", Pool: pool("a", "small", 1, 3)}}

	changes := planNodePoolChanges(current, []ClusterNodePoolModel{pool("a", "small", 1, 3)})

	assert.Empty(t, changes.Create)
	assert.Empty(t, changes.Update)
	assert.Empty(t, changes.Delete)
}

func TestPlanNodePoolChangesRemoveAll(t *testing.T) {
	current := []existingNodePool{{ID: "id-a", Pool: pool("a", "small", 1, 3)}}

	changes := planNodePoolChanges(current, nil)

	assert.Empty(t, changes.Create)
	assert.Equal(t, current, changes.Delete)
}

func TestOrderNodePools(t *testing.T) {
	fromAPI := []existingNodePool{
		{ID: "1", Pool: pool("zeta", "small", 1, 1)},
		{ID: "2", Pool: pool("added-in-console", "small", 1, 1)},
		{ID: "3", Pool: pool("alpha", "small", 1, 1)},
		{ID: "4", Pool: pool("beta", "small", 1, 1)},
	}
	prior := []ClusterNodePoolModel{pool("zeta", "small", 1, 1), pool("alpha", "small", 1, 1)}

	got := orderNodePools(fromAPI, prior)

	names := make([]string, 0, len(got))
	for _, p := range got {
		names = append(names, p.Name.ValueString())
	}
	assert.Equal(t, []string{"zeta", "alpha", "added-in-console", "beta"}, names)
}

func TestOrderNodePoolsImport(t *testing.T) {
	fromAPI := []existingNodePool{
		{ID: "1", Pool: pool("b", "small", 1, 1)},
		{ID: "2", Pool: pool("a", "small", 1, 1)},
	}

	got := orderNodePools(fromAPI, nil)

	assert.Equal(t, []ClusterNodePoolModel{pool("a", "small", 1, 1), pool("b", "small", 1, 1)}, got)
}

type fakeRegionsService struct {
	organizationv1connect.UnimplementedClusterServiceHandler
}

func (fakeRegionsService) ListRegions(_ context.Context, _ *organizationv1.ListRegionsRequest) (*organizationv1.ListRegionsResponse, error) {
	return organizationv1.ListRegionsResponse_builder{Regions: []*organizationv1.Region{
		organizationv1.Region_builder{Name: "local", MachineTypes: []*organizationv1.RegionMachineType{
			organizationv1.RegionMachineType_builder{Name: "local-small"}.Build(),
			organizationv1.RegionMachineType_builder{Name: "local-medium"}.Build(),
		}}.Build(),
	}}.Build(), nil
}

func TestCheckMachineTypes(t *testing.T) {
	path, handler := organizationv1connect.NewClusterServiceHandler(fakeRegionsService{})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	require.Equal(t, "/organization.v1.ClusterService/", path)
	client := &FundamentClient{ClusterService: organizationv1connect.NewClusterServiceClient(srv.Client(), srv.URL)}
	ctx := context.Background()

	assert.False(t, checkMachineTypes(ctx, client, "local", []ClusterNodePoolModel{pool("a", "local-medium", 1, 1)}).HasError())
	assert.False(t, checkMachineTypes(ctx, client, "nowhere", nil).HasError(), "nothing to create, nothing to check")

	diags := checkMachineTypes(ctx, client, "local", []ClusterNodePoolModel{pool("a", "local-small", 1, 1), pool("b", "n1-standard-1", 1, 1)})
	require.Len(t, diags.Errors(), 1)
	assert.Contains(t, diags.Errors()[0].Detail(), `Node pool "b": region "local" does not offer machine type "n1-standard-1"; it offers local-small, local-medium.`)

	diags = checkMachineTypes(ctx, client, "nowhere", []ClusterNodePoolModel{pool("a", "local-small", 1, 1)})
	require.Len(t, diags.Errors(), 1)
	assert.Contains(t, diags.Errors()[0].Detail(), `Region "nowhere" is not in the catalog.`)
}
