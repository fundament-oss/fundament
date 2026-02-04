package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestClusterNamespacesDataSourceModel(t *testing.T) {
	// Test that the model can be created with expected values
	model := ClusterNamespacesDataSourceModel{
		ID:        types.StringValue("test-cluster-id"),
		ClusterID: types.StringValue("test-cluster-id"),
		Namespaces: []NamespaceModel{
			{
				ID:        types.StringValue("namespace-1"),
				Name:      types.StringValue("ns-1"),
				ProjectID: types.StringValue("project-1"),
				ClusterID: types.StringValue("test-cluster-id"),
				CreatedAt: types.StringValue("2024-01-01T00:00:00Z"),
			},
			{
				ID:        types.StringValue("namespace-2"),
				Name:      types.StringValue("ns-2"),
				ProjectID: types.StringValue("project-2"),
				ClusterID: types.StringValue("test-cluster-id"),
				CreatedAt: types.StringValue("2024-01-02T00:00:00Z"),
			},
		},
	}

	if model.ID.ValueString() != "test-cluster-id" {
		t.Errorf("Expected ID 'test-cluster-id', got '%s'", model.ID.ValueString())
	}

	if model.ClusterID.ValueString() != "test-cluster-id" {
		t.Errorf("Expected cluster_id 'test-cluster-id', got '%s'", model.ClusterID.ValueString())
	}

	if len(model.Namespaces) != 2 {
		t.Errorf("Expected 2 namespaces, got %d", len(model.Namespaces))
	}

	if model.Namespaces[0].Name.ValueString() != "ns-1" {
		t.Errorf("Expected first namespace name 'ns-1', got '%s'", model.Namespaces[0].Name.ValueString())
	}

	if model.Namespaces[1].Name.ValueString() != "ns-2" {
		t.Errorf("Expected second namespace name 'ns-2', got '%s'", model.Namespaces[1].Name.ValueString())
	}
}
