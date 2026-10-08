package provider

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
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
	_, handler := organizationv1connect.NewClusterServiceHandler(fakeRegionsService{})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
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

// fakePoolService records the node pool calls applyNodePoolChanges makes.
type fakePoolService struct {
	fakeRegionsService
	calls      []string
	failCreate string
}

func (f *fakePoolService) ListRegions(ctx context.Context, req *organizationv1.ListRegionsRequest) (*organizationv1.ListRegionsResponse, error) {
	f.calls = append(f.calls, "ListRegions")
	return f.fakeRegionsService.ListRegions(ctx, req)
}

func (f *fakePoolService) CreateNodePool(ctx context.Context, req *organizationv1.CreateNodePoolRequest) (*organizationv1.CreateNodePoolResponse, error) {
	f.calls = append(f.calls, "Create "+req.GetName())
	if req.GetName() == f.failCreate {
		return nil, connect.NewError(connect.CodeInternal, errors.New("refused"))
	}
	if callInfo, ok := connect.CallInfoForHandlerContext(ctx); ok {
		callInfo.ResponseHeader().Set(idempotencyHeaderStatus, statusCompleted)
	}
	return organizationv1.CreateNodePoolResponse_builder{NodePoolId: "id-" + req.GetName()}.Build(), nil
}

func (f *fakePoolService) UpdateNodePool(_ context.Context, req *organizationv1.UpdateNodePoolRequest) (*organizationv1.UpdateNodePoolResponse, error) {
	f.calls = append(f.calls, "Update "+req.GetNodePoolId())
	return organizationv1.UpdateNodePoolResponse_builder{}.Build(), nil
}

func (f *fakePoolService) DeleteNodePool(_ context.Context, req *organizationv1.DeleteNodePoolRequest) (*organizationv1.DeleteNodePoolResponse, error) {
	f.calls = append(f.calls, "Delete "+req.GetNodePoolId())
	return organizationv1.DeleteNodePoolResponse_builder{}.Build(), nil
}

func newFakePoolClient(t *testing.T, service *fakePoolService) *FundamentClient {
	t.Helper()
	_, handler := organizationv1connect.NewClusterServiceHandler(service)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &FundamentClient{ClusterService: organizationv1connect.NewClusterServiceClient(srv.Client(), srv.URL)}
}

var applyCurrent = []existingNodePool{
	{ID: "id-keep", Pool: pool("keep", "local-small", 1, 3)},
	{ID: "id-resize", Pool: pool("resize", "local-small", 1, 3)},
	{ID: "id-retype", Pool: pool("retype", "local-small", 1, 3)},
	{ID: "id-gone", Pool: pool("gone", "local-small", 1, 3)},
}

func TestApplyNodePoolChangesOrder(t *testing.T) {
	service := &fakePoolService{}
	client := newFakePoolClient(t, service)

	diags := applyNodePoolChanges(context.Background(), client, "c1", "local", applyCurrent, []ClusterNodePoolModel{
		pool("keep", "local-small", 1, 3),
		pool("resize", "local-small", 2, 5),
		pool("retype", "local-medium", 1, 3),
		pool("new", "local-small", 1, 1),
	})

	require.False(t, diags.HasError(), "%v", diags)
	assert.Equal(t, []string{
		"ListRegions",      // machine types checked before anything changes
		"Delete id-retype", // the name must be free for its replacement
		"Create retype",
		"Create new",
		"Update id-resize",
		"Delete id-gone", // removed pools go last, so the cluster keeps workers in between
	}, service.calls)
}

func TestApplyNodePoolChangesRefusedMachineType(t *testing.T) {
	service := &fakePoolService{}
	client := newFakePoolClient(t, service)

	diags := applyNodePoolChanges(context.Background(), client, "c1", "local", applyCurrent, []ClusterNodePoolModel{
		pool("keep", "local-small", 1, 3),
		pool("retype", "local-huge", 1, 3),
	})

	require.True(t, diags.HasError())
	assert.Equal(t, []string{"ListRegions"}, service.calls, "nothing is deleted when a machine type is not offered")
}

func TestApplyNodePoolChangesStopsAtFirstError(t *testing.T) {
	service := &fakePoolService{failCreate: "new"}
	client := newFakePoolClient(t, service)

	diags := applyNodePoolChanges(context.Background(), client, "c1", "local", applyCurrent, []ClusterNodePoolModel{
		pool("keep", "local-small", 1, 3),
		pool("new", "local-small", 1, 1),
	})

	require.True(t, diags.HasError())
	assert.Equal(t, []string{"ListRegions", "Create new"}, service.calls, "no pool is removed after a failed create")
}

func TestNodePoolsValidator(t *testing.T) {
	poolType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"name":          types.StringType,
		"machine_type":  types.StringType,
		"autoscale_min": types.Int64Type,
		"autoscale_max": types.Int64Type,
	}}
	validate := func(pools ...ClusterNodePoolModel) []string {
		list, diags := types.ListValueFrom(context.Background(), poolType, pools)
		require.False(t, diags.HasError(), "%v", diags)
		resp := &validator.ListResponse{}
		nodePoolsValidator{}.ValidateList(context.Background(), validator.ListRequest{Path: path.Root("node_pool"), ConfigValue: list}, resp)
		summaries := make([]string, 0, len(resp.Diagnostics.Errors()))
		for _, d := range resp.Diagnostics.Errors() {
			summaries = append(summaries, d.Summary())
		}
		return summaries
	}

	assert.Empty(t, validate(pool("a", "local-small", 1, 3), pool("b", "local-small", 2, 2)))
	assert.Equal(t, []string{"Duplicate Node Pool Name"}, validate(pool("a", "local-small", 1, 3), pool("a", "local-medium", 1, 1)))
	assert.Equal(t, []string{"Invalid Node Pool Size"}, validate(pool("a", "local-small", 3, 2)))
}
