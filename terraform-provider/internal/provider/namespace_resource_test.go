package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestNamespaceModel_Resource(t *testing.T) {
	// Test that the model can be created with expected values
	model := NamespaceModel{
		ID:        types.StringValue("test-namespace-id"),
		Name:      types.StringValue("test-namespace"),
		ProjectID: types.StringValue("test-project-id"),
		ClusterID: types.StringValue("test-cluster-id"),
		Created:   types.StringValue("2024-01-01T00:00:00Z"),
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

	if model.ClusterID.ValueString() != "test-cluster-id" {
		t.Errorf("Expected cluster_id 'test-cluster-id', got '%s'", model.ClusterID.ValueString())
	}

	if model.Created.ValueString() != "2024-01-01T00:00:00Z" {
		t.Errorf("Expected created '2024-01-01T00:00:00Z', got '%s'", model.Created.ValueString())
	}
}

func TestNamespaceModel_NullValues(t *testing.T) {
	// Test that null values are handled correctly
	model := NamespaceModel{
		ID:        types.StringNull(),
		Name:      types.StringValue("test-namespace"),
		ProjectID: types.StringValue("test-project-id"),
		ClusterID: types.StringValue("test-cluster-id"),
		Created:   types.StringNull(),
	}

	if !model.ID.IsNull() {
		t.Error("Expected ID to be null")
	}

	if !model.Created.IsNull() {
		t.Error("Expected Created to be null")
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
