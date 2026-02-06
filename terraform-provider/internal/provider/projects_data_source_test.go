package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestProjectsDataSourceModel(t *testing.T) {
	// Test that the model can be created with expected values
	model := ProjectsDataSourceModel{
		ID: types.StringValue("projects"),
		Projects: []ProjectModel{
			{
				ID:      types.StringValue("project-1"),
				Name:    types.StringValue("test-project-1"),
				Created: types.StringValue("2024-01-15T10:30:00Z"),
			},
			{
				ID:      types.StringValue("project-2"),
				Name:    types.StringValue("test-project-2"),
				Created: types.StringValue("2024-01-16T11:45:00Z"),
			},
		},
	}

	if model.ID.ValueString() != "projects" {
		t.Errorf("Expected ID 'projects', got '%s'", model.ID.ValueString())
	}

	if len(model.Projects) != 2 {
		t.Errorf("Expected 2 projects, got %d", len(model.Projects))
	}

	if model.Projects[0].ID.ValueString() != "project-1" {
		t.Errorf("Expected first project ID 'project-1', got '%s'", model.Projects[0].ID.ValueString())
	}

	if model.Projects[0].Name.ValueString() != "test-project-1" {
		t.Errorf("Expected first project name 'test-project-1', got '%s'", model.Projects[0].Name.ValueString())
	}

	if model.Projects[1].ID.ValueString() != "project-2" {
		t.Errorf("Expected second project ID 'project-2', got '%s'", model.Projects[1].ID.ValueString())
	}
}
