package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestClusterDataSourceModel(t *testing.T) {
	// Test that the model can be created with expected values
	model := ClusterDataSourceModel{
		ID:                types.StringValue("test-id"),
		Name:              types.StringValue("test-cluster"),
		Region:            types.StringValue("eu-west-1"),
		KubernetesVersion: types.StringValue("1.28"),
		Status:            types.StringValue("running"),
	}

	if model.ID.ValueString() != "test-id" {
		t.Errorf("Expected ID 'test-id', got '%s'", model.ID.ValueString())
	}

	if model.Name.ValueString() != "test-cluster" {
		t.Errorf("Expected name 'test-cluster', got '%s'", model.Name.ValueString())
	}

	if model.Region.ValueString() != "eu-west-1" {
		t.Errorf("Expected region 'eu-west-1', got '%s'", model.Region.ValueString())
	}

	if model.KubernetesVersion.ValueString() != "1.28" {
		t.Errorf("Expected kubernetes_version '1.28', got '%s'", model.KubernetesVersion.ValueString())
	}

	if model.Status.ValueString() != "running" {
		t.Errorf("Expected status 'running', got '%s'", model.Status.ValueString())
	}
}
