package provider

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

func TestClusterResourceModel(t *testing.T) {
	// Test that the model can be created with expected values
	model := ClusterResourceModel{
		ID:                types.StringValue("test-id"),
		Name:              types.StringValue("test-cluster"),
		Region:            types.StringValue("eu-west-1"),
		KubernetesVersion: types.StringValue("1.28"),
		Status:            types.StringValue("running"),
	}

	if model.ID.ValueString() != "test-id" {
		t.Errorf("Expected ID 'test-id', got '%s'", model.ID.ValueString())
	}

	if model.Name.ValueString() != "test-cluster" {
		t.Errorf("Expected name 'test-cluster', got '%s'", model.Name.ValueString())
	}

	if model.Region.ValueString() != "eu-west-1" {
		t.Errorf("Expected region 'eu-west-1', got '%s'", model.Region.ValueString())
	}

	if model.KubernetesVersion.ValueString() != "1.28" {
		t.Errorf("Expected kubernetes_version '1.28', got '%s'", model.KubernetesVersion.ValueString())
	}

	if model.Status.ValueString() != "running" {
		t.Errorf("Expected status 'running', got '%s'", model.Status.ValueString())
	}
}

func TestClusterResourceModelNullValues(t *testing.T) {
	// Test that null values are handled correctly
	model := ClusterResourceModel{
		ID:                types.StringNull(),
		Name:              types.StringValue("test-cluster"),
		Region:            types.StringValue("eu-west-1"),
		KubernetesVersion: types.StringValue("1.28"),
		Status:            types.StringNull(),
	}

	if !model.ID.IsNull() {
		t.Error("Expected ID to be null")
	}

	if !model.Status.IsNull() {
		t.Error("Expected Status to be null")
	}

	if model.Name.IsNull() {
		t.Error("Expected Name to not be null")
	}
}

// fakeDeletingService answers GetCluster as the API does for a cluster whose
// deletion was requested, and lists the cluster with listedStatus.
type fakeDeletingService struct {
	organizationv1connect.UnimplementedClusterServiceHandler
	listedStatus organizationv1.ClusterStatus // UNSPECIFIED: not listed
}

func (fakeDeletingService) GetCluster(_ context.Context, _ *organizationv1.GetClusterRequest) (*organizationv1.GetClusterResponse, error) {
	return nil, connect.NewError(connect.CodePermissionDenied, errors.New("permission denied"))
}

func (f fakeDeletingService) ListClusters(_ context.Context, _ *organizationv1.ListClustersRequest) (*organizationv1.ListClustersResponse, error) {
	var clusters []*organizationv1.ListClustersResponse_ClusterSummary
	if f.listedStatus != organizationv1.ClusterStatus_CLUSTER_STATUS_UNSPECIFIED {
		clusters = append(clusters, organizationv1.ListClustersResponse_ClusterSummary_builder{Id: "c1", Status: f.listedStatus}.Build())
	}
	return organizationv1.ListClustersResponse_builder{Clusters: clusters}.Build(), nil
}

func TestClusterReadRefused(t *testing.T) {
	ctx := context.Background()
	r := &ClusterResource{}
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())

	read := func(t *testing.T, listed organizationv1.ClusterStatus) *resource.ReadResponse {
		t.Helper()
		_, handler := organizationv1connect.NewClusterServiceHandler(fakeDeletingService{listedStatus: listed})
		srv := httptest.NewServer(handler)
		t.Cleanup(srv.Close)
		r.client = &FundamentClient{ClusterService: organizationv1connect.NewClusterServiceClient(srv.Client(), srv.URL)}

		state := tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil)}
		require.False(t, state.Set(ctx, &ClusterResourceModel{
			ID:                types.StringValue("c1"),
			Name:              types.StringValue("web"),
			Region:            types.StringValue("local"),
			KubernetesVersion: types.StringValue("1.33.0"),
			Status:            types.StringValue("running"),
			NodePools:         []ClusterNodePoolModel{},
			Timeouts:          timeouts.Value{Object: types.ObjectNull(schemaResp.Schema.GetBlocks()["timeouts"].Type().(timeouts.Type).AttrTypes)},
		}).HasError())
		resp := &resource.ReadResponse{State: state}
		r.Read(ctx, resource.ReadRequest{State: state}, resp)
		return resp
	}

	t.Run("being deleted stays in state as deleting", func(t *testing.T) {
		resp := read(t, organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING)
		require.False(t, resp.Diagnostics.HasError())
		var status types.String
		require.False(t, resp.State.GetAttribute(ctx, path.Root("status"), &status).HasError())
		assert.Equal(t, "deleting", status.ValueString())
	})

	t.Run("gone leaves the state", func(t *testing.T) {
		resp := read(t, organizationv1.ClusterStatus_CLUSTER_STATUS_UNSPECIFIED)
		require.False(t, resp.Diagnostics.HasError())
		assert.True(t, resp.State.Raw.IsNull())
	})

	t.Run("refused while listed as running is an error", func(t *testing.T) {
		resp := read(t, organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), `Unable to read cluster "web": permission_denied: permission denied`)
	})
}
