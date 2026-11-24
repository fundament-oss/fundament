package provider

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

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

// fakeClusterService answers GetCluster with a cluster of status, or with
// getErr when that is set, and lists the cluster with listedStatus.
type fakeClusterService struct {
	organizationv1connect.UnimplementedClusterServiceHandler
	status       organizationv1.ClusterStatus
	getErr       error
	listedStatus organizationv1.ClusterStatus // UNSPECIFIED: not listed
}

func (f fakeClusterService) ListClusters(_ context.Context, _ *organizationv1.ListClustersRequest) (*organizationv1.ListClustersResponse, error) {
	var clusters []*organizationv1.ListClustersResponse_ClusterSummary
	if f.listedStatus != organizationv1.ClusterStatus_CLUSTER_STATUS_UNSPECIFIED {
		clusters = append(clusters, organizationv1.ListClustersResponse_ClusterSummary_builder{Id: "c1", Status: f.listedStatus}.Build())
	}
	return organizationv1.ListClustersResponse_builder{Clusters: clusters}.Build(), nil
}

func (f fakeClusterService) GetCluster(_ context.Context, _ *organizationv1.GetClusterRequest) (*organizationv1.GetClusterResponse, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return organizationv1.GetClusterResponse_builder{
		Cluster: organizationv1.ClusterDetails_builder{Id: "c1", Name: "web", Region: "local", KubernetesVersion: "1.33.0", Status: f.status}.Build(),
	}.Build(), nil
}

func (fakeClusterService) ListNodePools(_ context.Context, _ *organizationv1.ListNodePoolsRequest) (*organizationv1.ListNodePoolsResponse, error) {
	return organizationv1.ListNodePoolsResponse_builder{}.Build(), nil
}

func TestClusterRead(t *testing.T) {
	ctx := context.Background()
	r := &ClusterResource{}
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())

	read := func(t *testing.T, svc fakeClusterService) *resource.ReadResponse {
		t.Helper()
		_, handler := organizationv1connect.NewClusterServiceHandler(svc)
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
		resp := read(t, fakeClusterService{status: organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING})
		require.False(t, resp.Diagnostics.HasError())
		var status types.String
		require.False(t, resp.State.GetAttribute(ctx, path.Root("status"), &status).HasError())
		assert.Equal(t, "deleting", status.ValueString())
	})

	t.Run("gone leaves the state", func(t *testing.T) {
		resp := read(t, fakeClusterService{getErr: connect.NewError(connect.CodeNotFound, errors.New("cluster not found"))})
		require.False(t, resp.Diagnostics.HasError())
		assert.True(t, resp.State.Raw.IsNull())
	})

	denied := connect.NewError(connect.CodePermissionDenied, errors.New("permission denied"))

	// A server from before fundament#707 refuses a cluster as soon as its
	// deletion is requested; the list still shows it.
	t.Run("refused while listed as deleting stays in state as deleting", func(t *testing.T) {
		resp := read(t, fakeClusterService{getErr: denied, listedStatus: organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING})
		require.False(t, resp.Diagnostics.HasError())
		var status types.String
		require.False(t, resp.State.GetAttribute(ctx, path.Root("status"), &status).HasError())
		assert.Equal(t, "deleting", status.ValueString())
	})

	t.Run("refused and not listed leaves the state", func(t *testing.T) {
		resp := read(t, fakeClusterService{getErr: denied})
		require.False(t, resp.Diagnostics.HasError())
		assert.True(t, resp.State.Raw.IsNull())
	})

	t.Run("refused while listed as running is an error", func(t *testing.T) {
		resp := read(t, fakeClusterService{getErr: denied, listedStatus: organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING})
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), `Unable to read cluster "web": permission_denied: permission denied`)
	})
}

// waitForClusterDeleted follows the cluster on GetCluster: deleting until
// Gardener has removed it, then not found.
func TestWaitForClusterDeleted(t *testing.T) {
	prev := clusterPollInterval
	clusterPollInterval = time.Millisecond
	t.Cleanup(func() { clusterPollInterval = prev })

	client := func(t *testing.T, h organizationv1connect.ClusterServiceHandler) *FundamentClient {
		t.Helper()
		_, handler := organizationv1connect.NewClusterServiceHandler(h)
		srv := httptest.NewServer(handler)
		t.Cleanup(srv.Close)
		return &FundamentClient{ClusterService: organizationv1connect.NewClusterServiceClient(srv.Client(), srv.URL)}
	}

	t.Run("waits while deleting, done when not found", func(t *testing.T) {
		svc := &countdownClusterService{deletingFor: 3}
		require.NoError(t, waitForClusterDeleted(t.Context(), client(t, svc), "c1"))
		assert.Equal(t, 4, svc.calls)
	})

	t.Run("refused while listed as running stops the wait", func(t *testing.T) {
		svc := fakeClusterService{
			getErr:       connect.NewError(connect.CodePermissionDenied, errors.New("permission denied")),
			listedStatus: organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING,
		}
		err := waitForClusterDeleted(t.Context(), client(t, svc), "c1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied")
	})

	// A server from before fundament#707 refuses the cluster at once; the wait
	// follows the list until it no longer shows it.
	t.Run("older server: refused while listed, done when no longer listed", func(t *testing.T) {
		svc := &refusingListService{listedFor: 2}
		require.NoError(t, waitForClusterDeleted(t.Context(), client(t, svc), "c1"))
		assert.Equal(t, 3, svc.lists)
	})
}

// refusingListService refuses GetCluster as a server from before fundament#707
// does, and lists the cluster as deleting listedFor times.
type refusingListService struct {
	organizationv1connect.UnimplementedClusterServiceHandler
	listedFor int
	lists     int
}

func (*refusingListService) GetCluster(_ context.Context, _ *organizationv1.GetClusterRequest) (*organizationv1.GetClusterResponse, error) {
	return nil, connect.NewError(connect.CodePermissionDenied, errors.New("permission denied"))
}

func (s *refusingListService) ListClusters(_ context.Context, _ *organizationv1.ListClustersRequest) (*organizationv1.ListClustersResponse, error) {
	s.lists++
	var clusters []*organizationv1.ListClustersResponse_ClusterSummary
	if s.lists <= s.listedFor {
		clusters = append(clusters, organizationv1.ListClustersResponse_ClusterSummary_builder{Id: "c1", Status: organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING}.Build())
	}
	return organizationv1.ListClustersResponse_builder{Clusters: clusters}.Build(), nil
}

// countdownClusterService returns the cluster as deleting deletingFor times,
// then answers not found.
type countdownClusterService struct {
	organizationv1connect.UnimplementedClusterServiceHandler
	deletingFor int
	calls       int
}

// ListClusters lists nothing: once GetCluster answers not found, a server with
// fundament#707 no longer lists the cluster either.
func (*countdownClusterService) ListClusters(_ context.Context, _ *organizationv1.ListClustersRequest) (*organizationv1.ListClustersResponse, error) {
	return organizationv1.ListClustersResponse_builder{}.Build(), nil
}

func (c *countdownClusterService) GetCluster(_ context.Context, _ *organizationv1.GetClusterRequest) (*organizationv1.GetClusterResponse, error) {
	c.calls++
	if c.calls > c.deletingFor {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("cluster not found"))
	}
	return organizationv1.GetClusterResponse_builder{
		Cluster: organizationv1.ClusterDetails_builder{Id: "c1", Status: organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING}.Build(),
	}.Build(), nil
}

// Terraform shows a renamed node_pool block as an in-place change of its name;
// the plan carries a warning that the pool and its nodes are replaced.
func TestClusterModifyPlanWarnsOnReplacedNodePools(t *testing.T) {
	ctx := context.Background()
	r := &ClusterResource{}
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())
	s := schemaResp.Schema
	model := func(pools ...ClusterNodePoolModel) *ClusterResourceModel {
		return &ClusterResourceModel{
			ID:                types.StringValue("c1"),
			Name:              types.StringValue("web"),
			Region:            types.StringValue("local"),
			KubernetesVersion: types.StringValue("1.33.0"),
			Status:            types.StringValue("running"),
			NodePools:         pools,
			Timeouts:          timeouts.Value{Object: types.ObjectNull(s.GetBlocks()["timeouts"].Type().(timeouts.Type).AttrTypes)},
		}
	}
	modify := func(t *testing.T, current, planned *ClusterResourceModel) *resource.ModifyPlanResponse {
		t.Helper()
		state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
		require.False(t, state.Set(ctx, current).HasError())
		plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
		require.False(t, plan.Set(ctx, planned).HasError())
		resp := &resource.ModifyPlanResponse{Plan: plan}
		r.ModifyPlan(ctx, resource.ModifyPlanRequest{State: state, Plan: plan}, resp)
		require.False(t, resp.Diagnostics.HasError())
		return resp
	}

	t.Run("rename", func(t *testing.T) {
		resp := modify(t, model(pool("workers", "local-small", 1, 3)), model(pool("renamed", "local-small", 1, 3)))
		warnings := resp.Diagnostics.Warnings()
		require.Len(t, warnings, 1)
		assert.Equal(t, "Node Pools Removed or Replaced", warnings[0].Summary())
		assert.Contains(t, warnings[0].Detail(), `Node pool "workers" is removed`)
		assert.Contains(t, warnings[0].Detail(), "renaming one removes it and creates a new one")
		assert.Contains(t, warnings[0].Detail(), "the cluster keeps its current node pools")
	})

	t.Run("resize", func(t *testing.T) {
		resp := modify(t, model(pool("workers", "local-small", 1, 3)), model(pool("workers", "local-small", 2, 5)))
		assert.Empty(t, resp.Diagnostics.Warnings())
	})

	t.Run("machine type change", func(t *testing.T) {
		resp := modify(t, model(pool("workers", "local-small", 1, 3)), model(pool("workers", "local-medium", 1, 3)))
		warnings := resp.Diagnostics.Warnings()
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0].Detail(), `Node pool "workers" changes machine type from local-small to local-medium`)
	})

	// The whole cluster is replaced: a warning about one of its pools would
	// only distract from that.
	t.Run("cluster being deleted", func(t *testing.T) {
		current := model(pool("workers", "local-small", 1, 3))
		current.Status = types.StringValue(clusterStatusToString(organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING))
		resp := modify(t, current, model(pool("renamed", "local-small", 1, 3)))
		assert.Empty(t, resp.Diagnostics.Warnings())
		assert.Equal(t, path.Paths{path.Root("status")}, resp.RequiresReplace)
	})

	// A state from before the node_pool block existed holds a null list.
	t.Run("no node_pool in state", func(t *testing.T) {
		current := model()
		current.NodePools = nil
		state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
		require.False(t, state.Set(ctx, current).HasError())
		var list types.List
		require.False(t, state.GetAttribute(ctx, path.Root("node_pool"), &list).HasError())
		require.True(t, list.IsNull(), "the state must hold a null node_pool for this case")

		plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
		require.False(t, plan.Set(ctx, model(pool("workers", "local-small", 1, 3))).HasError())
		resp := &resource.ModifyPlanResponse{Plan: plan}
		r.ModifyPlan(ctx, resource.ModifyPlanRequest{State: state, Plan: plan}, resp)
		assert.False(t, resp.Diagnostics.HasError())
		assert.Empty(t, resp.Diagnostics.Warnings())
	})
}
