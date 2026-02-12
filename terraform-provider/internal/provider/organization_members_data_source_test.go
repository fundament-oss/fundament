package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestOrganizationMembersDataSourceModel(t *testing.T) {
	model := OrganizationMembersDataSourceModel{
		ID: types.StringValue("organization_members"),
		Members: []OrganizationMemberModel{
			{
				ID:         types.StringValue("member-1"),
				Email:      types.StringValue("admin@example.com"),
				Name:       types.StringValue("Admin User"),
				ExternalID: types.StringValue("auth0|admin"),
				Role:       types.StringValue("admin"),
				Created:    types.StringValue("2024-01-15T10:30:00Z"),
			},
			{
				ID:         types.StringValue("member-2"),
				Email:      types.StringValue("viewer@example.com"),
				Name:       types.StringValue("viewer@example.com"),
				ExternalID: types.StringNull(),
				Role:       types.StringValue("viewer"),
				Created:    types.StringValue("2024-02-01T14:00:00Z"),
			},
		},
	}

	if model.ID.ValueString() != "organization_members" {
		t.Errorf("Expected ID 'organization_members', got '%s'", model.ID.ValueString())
	}

	if len(model.Members) != 2 {
		t.Fatalf("Expected 2 members, got %d", len(model.Members))
	}

	if model.Members[0].Role.ValueString() != "admin" {
		t.Errorf("Expected first member role 'admin', got '%s'", model.Members[0].Role.ValueString())
	}

	if !model.Members[1].ExternalID.IsNull() {
		t.Error("Expected second member ExternalID to be null")
	}
}
