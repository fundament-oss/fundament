package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestProjectMembersDataSourceModel(t *testing.T) {
	// Test that the model can be created with expected values
	model := ProjectMembersDataSourceModel{
		ProjectID: types.StringValue("test-project-id"),
		Members: []ProjectMemberModel{
			{
				ID:        types.StringValue("member-1"),
				ProjectID: types.StringValue("test-project-id"),
				UserID:    types.StringValue("user-1"),
				UserName:  types.StringValue("User One"),
				Role:      types.StringValue("admin"),
				Created:   types.StringValue("2024-01-15T10:30:00Z"),
			},
			{
				ID:        types.StringValue("member-2"),
				ProjectID: types.StringValue("test-project-id"),
				UserID:    types.StringValue("user-2"),
				UserName:  types.StringValue("User Two"),
				Role:      types.StringValue("viewer"),
				Created:   types.StringValue("2024-01-16T11:45:00Z"),
			},
		},
	}

	if model.ProjectID.ValueString() != "test-project-id" {
		t.Errorf("Expected ProjectID 'test-project-id', got '%s'", model.ProjectID.ValueString())
	}

	if len(model.Members) != 2 {
		t.Errorf("Expected 2 members, got %d", len(model.Members))
	}

	if model.Members[0].ID.ValueString() != "member-1" {
		t.Errorf("Expected first member ID 'member-1', got '%s'", model.Members[0].ID.ValueString())
	}

	if model.Members[0].Role.ValueString() != "admin" {
		t.Errorf("Expected first member role 'admin', got '%s'", model.Members[0].Role.ValueString())
	}

	if model.Members[1].ID.ValueString() != "member-2" {
		t.Errorf("Expected second member ID 'member-2', got '%s'", model.Members[1].ID.ValueString())
	}

	if model.Members[1].Role.ValueString() != "viewer" {
		t.Errorf("Expected second member role 'viewer', got '%s'", model.Members[1].Role.ValueString())
	}
}
