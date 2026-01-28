package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestProjectDataSourceModel(t *testing.T) {
	// Test that the model can be created with expected values
	model := ProjectDataSourceModel{
		ID:        types.StringValue("test-id"),
		Name:      types.StringValue("test-project"),
		CreatedAt: types.StringValue("2024-01-15T10:30:00Z"),
	}

	if model.ID.ValueString() != "test-id" {
		t.Errorf("Expected ID 'test-id', got '%s'", model.ID.ValueString())
	}

	if model.Name.ValueString() != "test-project" {
		t.Errorf("Expected name 'test-project', got '%s'", model.Name.ValueString())
	}

	if model.CreatedAt.ValueString() != "2024-01-15T10:30:00Z" {
		t.Errorf("Expected created_at '2024-01-15T10:30:00Z', got '%s'", model.CreatedAt.ValueString())
	}
}
