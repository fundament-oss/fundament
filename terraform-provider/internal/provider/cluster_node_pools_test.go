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

// fakePoolService records the SetNodePools requests setNodePools makes.
type fakePoolService struct {
	fakeRegionsService
	requests []*organizationv1.SetNodePoolsRequest
	refuse   error
	// refuseTimes limits the refusal to the first n calls; 0 means every call.
	refuseTimes int
}

func (f *fakePoolService) SetNodePools(_ context.Context, req *organizationv1.SetNodePoolsRequest) (*organizationv1.SetNodePoolsResponse, error) {
	f.requests = append(f.requests, req)
	if f.refuse != nil && (f.refuseTimes == 0 || len(f.requests) <= f.refuseTimes) {
		return nil, f.refuse
	}
	return organizationv1.SetNodePoolsResponse_builder{}.Build(), nil
}

func newFakePoolClient(t *testing.T, service *fakePoolService) *FundamentClient {
	t.Helper()
	_, handler := organizationv1connect.NewClusterServiceHandler(service)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &FundamentClient{ClusterService: organizationv1connect.NewClusterServiceClient(srv.Client(), srv.URL)}
}

// The planned blocks go to the API as one complete set, in their order.
func TestSetNodePoolsSendsTheWholeSet(t *testing.T) {
	service := &fakePoolService{}
	client := newFakePoolClient(t, service)

	diags := setNodePools(context.Background(), client, "c1", []ClusterNodePoolModel{
		pool("workers", "local-medium", 1, 3),
		pool("spare", "local-small", 0, 0),
	})

	require.False(t, diags.HasError(), "%v", diags)
	require.Len(t, service.requests, 1)
	req := service.requests[0]
	assert.Equal(t, "c1", req.GetClusterId())
	require.Len(t, req.GetNodePools(), 2)
	assert.Equal(t, "workers", req.GetNodePools()[0].GetName())
	assert.Equal(t, "local-medium", req.GetNodePools()[0].GetMachineType())
	assert.Equal(t, int32(1), req.GetNodePools()[0].GetAutoscaleMin())
	assert.Equal(t, int32(3), req.GetNodePools()[0].GetAutoscaleMax())
	assert.Equal(t, "spare", req.GetNodePools()[1].GetName())
	assert.Equal(t, "local-small", req.GetNodePools()[1].GetMachineType())
	assert.Equal(t, int32(0), req.GetNodePools()[1].GetAutoscaleMin())
	assert.Equal(t, int32(0), req.GetNodePools()[1].GetAutoscaleMax())
}

func TestSetNodePoolsReportsARefusal(t *testing.T) {
	service := &fakePoolService{refuse: connect.NewError(connect.CodeResourceExhausted, errors.New("over the organization's quota of 3"))}
	client := newFakePoolClient(t, service)

	diags := setNodePools(context.Background(), client, "c1", []ClusterNodePoolModel{pool("workers", "local-small", 1, 4)})

	require.True(t, diags.HasError())
	assert.Equal(t, "Unable to Set Node Pools", diags.Errors()[0].Summary())
	assert.Contains(t, diags.Errors()[0].Detail(), "over the organization's quota of 3")
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
	assert.Empty(t, validate(pool("a", "local-small", 1, 3), pool("spare", "local-small", 0, 0)), "an empty pool next to one that runs nodes")
	assert.Equal(t, []string{"No Node Pool Can Run Nodes"}, validate(pool("a", "local-small", 0, 0), pool("b", "local-small", 0, 0)))

	resp := &validator.ListResponse{}
	nodePoolsValidator{}.ValidateList(context.Background(), validator.ListRequest{Path: path.Root("node_pool"), ConfigValue: types.ListNull(poolType)}, resp)
	require.Len(t, resp.Diagnostics.Errors(), 1)
	assert.Equal(t, "Missing Node Pool", resp.Diagnostics.Errors()[0].Summary())
}

// Only pools that are new or change machine type need the region to offer
// their type: a legacy pool keeps running and must not block an upgrade.
func TestNewOrRetyped(t *testing.T) {
	current := []ClusterNodePoolModel{pool("legacy", "n1-retired", 1, 3), pool("keep", "local-small", 1, 3)}
	planned := []ClusterNodePoolModel{
		pool("legacy", "n1-retired", 1, 3),
		pool("keep", "local-medium", 1, 3),
		pool("new", "local-small", 1, 1),
	}
	var names []string
	for _, p := range newOrRetyped(current, planned) {
		names = append(names, p.Name.ValueString())
	}
	assert.Equal(t, []string{"keep", "new"}, names)
}

func TestSameNodePools(t *testing.T) {
	a := []ClusterNodePoolModel{pool("a", "local-small", 1, 3), pool("b", "local-small", 0, 1)}
	assert.True(t, sameNodePools(a, []ClusterNodePoolModel{pool("b", "local-small", 0, 1), pool("a", "local-small", 1, 3)}), "order does not matter")
	assert.False(t, sameNodePools(a, []ClusterNodePoolModel{pool("a", "local-small", 1, 4), pool("b", "local-small", 0, 1)}), "a resize")
	assert.False(t, sameNodePools(a, []ClusterNodePoolModel{pool("a", "local-medium", 1, 3), pool("b", "local-small", 0, 1)}), "a new machine type")
	assert.False(t, sameNodePools(a, a[:1]), "a removed pool")
}

// A server that cannot be reached is asked again; a refusal is not.
func TestSetNodePoolsRetriesWhileTheServerIsUnavailable(t *testing.T) {
	service := &fakePoolService{refuse: connect.NewError(connect.CodeUnavailable, errors.New("connection reset")), refuseTimes: 2}
	client := newFakePoolClient(t, service)

	diags := setNodePools(context.Background(), client, "c1", []ClusterNodePoolModel{pool("workers", "local-small", 1, 4)})

	require.False(t, diags.HasError(), "%v", diags)
	assert.Len(t, service.requests, 3, "two failed tries, then the one that went through")
}

func TestRetryTransientReturnsARefusalAtOnce(t *testing.T) {
	calls := 0
	err := retryTransient(context.Background(), defaultClock, func(context.Context) error {
		calls++
		return connect.NewError(connect.CodeInvalidArgument, errors.New("bad"))
	})
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Equal(t, 1, calls)
}
