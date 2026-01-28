package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestProjectNamespacesDataSourceModel(t *testing.T) {
	// Test that the model can be created with expected values
	model := ProjectNamespacesDataSourceModel{
		ID:        types.StringValue("project-namespaces-test-project-id"),
		ProjectID: types.StringValue("test-project-id"),
		Namespaces: []ProjectNamespaceModel{
			{
				ID:        types.StringValue("namespace-1"),
				Name:      types.StringValue("ns-1"),
				ClusterID: types.StringValue("cluster-1"),
				CreatedAt: types.StringValue("2024-01-01T00:00:00Z"),
			},
			{
				ID:        types.StringValue("namespace-2"),
				Name:      types.StringValue("ns-2"),
				ClusterID: types.StringValue("cluster-2"),
				CreatedAt: types.StringValue("2024-01-02T00:00:00Z"),
			},
		},
	}

	if model.ID.ValueString() != "project-namespaces-test-project-id" {
		t.Errorf("Expected ID 'project-namespaces-test-project-id', got '%s'", model.ID.ValueString())
	}

	if model.ProjectID.ValueString() != "test-project-id" {
		t.Errorf("Expected project_id 'test-project-id', got '%s'", model.ProjectID.ValueString())
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

	if model.Namespaces[0].ClusterID.ValueString() != "cluster-1" {
		t.Errorf("Expected first namespace cluster_id 'cluster-1', got '%s'", model.Namespaces[0].ClusterID.ValueString())
	}
}
