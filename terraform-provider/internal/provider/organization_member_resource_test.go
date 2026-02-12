package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestOrganizationMemberModel_Resource(t *testing.T) {
	model := OrganizationMemberModel{
		ID:         types.StringValue("test-id"),
		Email:      types.StringValue("user@example.com"),
		Name:       types.StringValue("user@example.com"),
		ExternalID: types.StringNull(),
		Role:       types.StringValue("admin"),
		Created:    types.StringValue("2024-01-15T10:30:00Z"),
	}

	if model.ID.ValueString() != "test-id" {
		t.Errorf("Expected ID 'test-id', got '%s'", model.ID.ValueString())
	}

	if model.Email.ValueString() != "user@example.com" {
		t.Errorf("Expected Email 'user@example.com', got '%s'", model.Email.ValueString())
	}

	if model.Name.ValueString() != "user@example.com" {
		t.Errorf("Expected Name 'user@example.com', got '%s'", model.Name.ValueString())
	}

	if !model.ExternalID.IsNull() {
		t.Error("Expected ExternalID to be null")
	}

	if model.Role.ValueString() != "admin" {
		t.Errorf("Expected Role 'admin', got '%s'", model.Role.ValueString())
	}

	if model.Created.ValueString() != "2024-01-15T10:30:00Z" {
		t.Errorf("Expected Created '2024-01-15T10:30:00Z', got '%s'", model.Created.ValueString())
	}
}

func TestOrganizationMemberModelNullValues(t *testing.T) {
	model := OrganizationMemberModel{
		ID:         types.StringNull(),
		Email:      types.StringValue("user@example.com"),
		Name:       types.StringNull(),
		ExternalID: types.StringNull(),
		Role:       types.StringValue("viewer"),
		Created:    types.StringNull(),
	}

	if !model.ID.IsNull() {
		t.Error("Expected ID to be null")
	}

	if !model.Name.IsNull() {
		t.Error("Expected Name to be null")
	}

	if !model.ExternalID.IsNull() {
		t.Error("Expected ExternalID to be null")
	}

	if !model.Created.IsNull() {
		t.Error("Expected Created to be null")
	}

	if model.Email.IsNull() {
		t.Error("Expected Email to not be null")
	}

	if model.Role.IsNull() {
		t.Error("Expected Role to not be null")
	}
}

func TestOrganizationMemberModelWithExternalID(t *testing.T) {
	model := OrganizationMemberModel{
		Name:       types.StringValue("John Doe"),
		ExternalID: types.StringValue("auth0|12345"),
	}

	if model.ExternalID.IsNull() {
		t.Error("Expected ExternalID to not be null")
	}

	if model.ExternalID.ValueString() != "auth0|12345" {
		t.Errorf("Expected ExternalID 'auth0|12345', got '%s'", model.ExternalID.ValueString())
	}

	if model.Name.ValueString() != "John Doe" {
		t.Errorf("Expected Name 'John Doe', got '%s'", model.Name.ValueString())
	}
}
