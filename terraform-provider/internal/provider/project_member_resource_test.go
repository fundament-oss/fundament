package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

var (
	testMemberID  = "019b4000-1000-7000-8000-000000000001"
	testProjectID = "019b4000-2000-7000-8000-000000000001"
	testUserID    = "019b4000-3000-7000-8000-000000000001"
)

func TestProjectMemberModel_Resource(t *testing.T) {
	model := ProjectMemberModel{
		ID:        types.StringValue(testMemberID),
		ProjectID: types.StringValue(testProjectID),
		UserID:    types.StringValue(testUserID),
		UserName:  types.StringValue("test-user"),
		Role:      types.StringValue("admin"),
		Created:   types.StringValue("2024-01-15T10:30:00Z"),
	}

	if model.ID.ValueString() != testMemberID {
		t.Errorf("Expected ID %q, got %q", testMemberID, model.ID.ValueString())
	}

	if model.ProjectID.ValueString() != testProjectID {
		t.Errorf("Expected ProjectID %q, got %q", testProjectID, model.ProjectID.ValueString())
	}

	if model.UserID.ValueString() != testUserID {
		t.Errorf("Expected UserID %q, got %q", testUserID, model.UserID.ValueString())
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
	model := ProjectMemberModel{
		ID:        types.StringNull(),
		ProjectID: types.StringValue(testProjectID),
		UserID:    types.StringValue(testUserID),
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
	}

	for _, tt := range tests {
		result := projectMemberRoleToProto(tt.input)
		if result != tt.expected {
			t.Errorf("projectMemberRoleToProto(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestProjectMemberRoleToProto_PanicsOnUnknown(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for unknown role, got none")
		}
	}()
	projectMemberRoleToProto("unknown")
}

func TestProjectMemberRoleToString(t *testing.T) {
	tests := []struct {
		input    organizationv1.ProjectMemberRole
		expected string
	}{
		{organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_ADMIN, "admin"},
		{organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER, "viewer"},
	}

	for _, tt := range tests {
		result := projectMemberRoleToString(tt.input)
		if result != tt.expected {
			t.Errorf("projectMemberRoleToString(%v) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestProjectMemberRoleToString_PanicsOnUnknown(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for unknown role proto value, got none")
		}
	}()
	projectMemberRoleToString(organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_UNSPECIFIED)
}
