package provider

import (
	"testing"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestProjectMemberModel_Resource(t *testing.T) {
	// Test that the model can be created with expected values
	model := ProjectMemberModel{
		ID:        types.StringValue("test-member-id"),
		ProjectID: types.StringValue("test-project-id"),
		UserID:    types.StringValue("test-user-id"),
		UserName:  types.StringValue("test-user"),
		Role:      types.StringValue("admin"),
		Created:   types.StringValue("2024-01-15T10:30:00Z"),
	}

	if model.ID.ValueString() != "test-member-id" {
		t.Errorf("Expected ID 'test-member-id', got '%s'", model.ID.ValueString())
	}

	if model.ProjectID.ValueString() != "test-project-id" {
		t.Errorf("Expected ProjectID 'test-project-id', got '%s'", model.ProjectID.ValueString())
	}

	if model.UserID.ValueString() != "test-user-id" {
		t.Errorf("Expected UserID 'test-user-id', got '%s'", model.UserID.ValueString())
	}

	if model.UserName.ValueString() != "test-user" {
		t.Errorf("Expected UserName 'test-user', got '%s'", model.UserName.ValueString())
	}

	if model.Role.ValueString() != "admin" {
		t.Errorf("Expected Role 'admin', got '%s'", model.Role.ValueString())
	}

	if model.Created.ValueString() != "2024-01-15T10:30:00Z" {
		t.Errorf("Expected Created '2024-01-15T10:30:00Z', got '%s'", model.Created.ValueString())
	}
}

func TestProjectMemberModel_NullValues(t *testing.T) {
	// Test that null values are handled correctly
	model := ProjectMemberModel{
		ID:        types.StringNull(),
		ProjectID: types.StringValue("test-project-id"),
		UserID:    types.StringValue("test-user-id"),
		UserName:  types.StringNull(),
		Role:      types.StringValue("viewer"),
		Created:   types.StringNull(),
	}

	if !model.ID.IsNull() {
		t.Error("Expected ID to be null")
	}

	if !model.UserName.IsNull() {
		t.Error("Expected UserName to be null")
	}

	if !model.Created.IsNull() {
		t.Error("Expected Created to be null")
	}

	if model.ProjectID.IsNull() {
		t.Error("Expected ProjectID to not be null")
	}

	if model.UserID.IsNull() {
		t.Error("Expected UserID to not be null")
	}

	if model.Role.IsNull() {
		t.Error("Expected Role to not be null")
	}
}

func TestProjectMemberRoleToProto(t *testing.T) {
	tests := []struct {
		input    string
		expected organizationv1.ProjectMemberRole
	}{
		{"admin", organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_ADMIN},
		{"viewer", organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER},
		{"", organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_UNSPECIFIED},
		{"unknown", organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_UNSPECIFIED},
	}

	for _, tt := range tests {
		result := projectMemberRoleToProto(tt.input)
		if result != tt.expected {
			t.Errorf("projectMemberRoleToProto(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestProjectMemberRoleToString(t *testing.T) {
	tests := []struct {
		input    organizationv1.ProjectMemberRole
		expected string
	}{
		{organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_ADMIN, "admin"},
		{organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER, "viewer"},
		{organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_UNSPECIFIED, ""},
	}

	for _, tt := range tests {
		result := projectMemberRoleToString(tt.input)
		if result != tt.expected {
			t.Errorf("projectMemberRoleToString(%v) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
