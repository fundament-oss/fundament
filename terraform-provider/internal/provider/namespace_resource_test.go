package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestNamespaceModel_Resource(t *testing.T) {
	// Test that the model can be created with expected values
	model := NamespaceResourceModel{
		ID:          types.StringValue("test-namespace-id"),
		Name:        types.StringValue("test-namespace"),
		ProjectID:   types.StringValue("test-project-id"),
		ProjectName: types.StringValue("test-project"),
		ClusterID:   types.StringValue("test-cluster-id"),
		ClusterName: types.StringValue("test-cluster"),
		Created:     types.StringValue("2024-01-01T00:00:00Z"),
	}

	if model.ID.ValueString() != "test-namespace-id" {
		t.Errorf("Expected ID 'test-namespace-id', got '%s'", model.ID.ValueString())
	}

	if model.Name.ValueString() != "test-namespace" {
		t.Errorf("Expected name 'test-namespace', got '%s'", model.Name.ValueString())
	}

	if model.ProjectID.ValueString() != "test-project-id" {
		t.Errorf("Expected project_id 'test-project-id', got '%s'", model.ProjectID.ValueString())
	}

	if model.ProjectName.ValueString() != "test-project" {
		t.Errorf("Expected project_name 'test-project', got '%s'", model.ProjectName.ValueString())
	}

	if model.ClusterID.ValueString() != "test-cluster-id" {
		t.Errorf("Expected cluster_id 'test-cluster-id', got '%s'", model.ClusterID.ValueString())
	}

	if model.ClusterName.ValueString() != "test-cluster" {
		t.Errorf("Expected cluster_name 'test-cluster', got '%s'", model.ClusterName.ValueString())
	}

	if model.Created.ValueString() != "2024-01-01T00:00:00Z" {
		t.Errorf("Expected created '2024-01-01T00:00:00Z', got '%s'", model.Created.ValueString())
	}
}

func TestNamespaceModel_NullValues(t *testing.T) {
	// Test that null values are handled correctly
	model := NamespaceResourceModel{
		ID:          types.StringNull(),
		Name:        types.StringValue("test-namespace"),
		ProjectID:   types.StringValue("test-project-id"),
		ProjectName: types.StringNull(),
		ClusterID:   types.StringValue("test-cluster-id"),
		ClusterName: types.StringNull(),
		Created:     types.StringNull(),
	}

	if !model.ID.IsNull() {
		t.Error("Expected ID to be null")
	}

	if !model.Created.IsNull() {
		t.Error("Expected Created to be null")
	}

	if !model.ProjectName.IsNull() {
		t.Error("Expected ProjectName to be null")
	}

	if !model.ClusterName.IsNull() {
		t.Error("Expected ClusterName to be null")
	}

	if model.Name.IsNull() {
		t.Error("Expected Name to not be null")
	}

	if model.ProjectID.IsNull() {
		t.Error("Expected ProjectID to not be null")
	}

	if model.ClusterID.IsNull() {
		t.Error("Expected ClusterID to not be null")
	}
}
