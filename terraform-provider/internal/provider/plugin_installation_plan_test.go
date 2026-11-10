package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pluginInstallationSchema(t *testing.T) schema.Schema {
	t.Helper()
	var resp resource.SchemaResponse
	(&PluginInstallationResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	require.False(t, resp.Diagnostics.HasError())
	return resp.Schema
}

// A placeholder default would make the plan hold a known "unknown", which
// Create may not replace with the resolved version.
func TestPluginInstallationSchema_PinHasNoPlaceholderDefault(t *testing.T) {
	s := pluginInstallationSchema(t)
	for _, name := range []string{"plugin_version", "definition_hash"} {
		attr, ok := s.Attributes[name].(schema.StringAttribute)
		require.True(t, ok, name)
		assert.Nil(t, attr.Default, name)
		assert.True(t, attr.Optional && attr.Computed, name)
	}
}

func TestPinFromPlan(t *testing.T) {
	tests := []struct {
		name          string
		version, hash types.String
		wantVersion   string
		wantHash      string
		wantResolve   bool
	}{
		{"unset", types.StringUnknown(), types.StringUnknown(), "", "", true},
		{"placeholder", types.StringValue(unpinnedPluginVersion), types.StringValue(unpinnedDefinitionHash), "", "", true},
		{"version only", types.StringValue("1.2.3"), types.StringUnknown(), "1.2.3", unpinnedDefinitionHash, false},
		{"pinned", types.StringValue("1.2.3"), types.StringValue("sha256:abc"), "1.2.3", "sha256:abc", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, hash, resolve := pinFromPlan(tt.version, tt.hash)
			assert.Equal(t, tt.wantResolve, resolve)
			if !resolve {
				assert.Equal(t, tt.wantVersion, version)
				assert.Equal(t, tt.wantHash, hash)
			}
		})
	}
}

// createPlan builds a create plan with plugin_version set to version.
func createPlan(t *testing.T, s schema.Schema, version tftypes.Value) tfsdk.Plan {
	t.Helper()
	objType := s.Type().TerraformType(context.Background()).(tftypes.Object)
	vals := map[string]tftypes.Value{}
	for name, typ := range objType.AttributeTypes {
		vals[name] = tftypes.NewValue(typ, tftypes.UnknownValue)
	}
	vals["cluster_id"] = tftypes.NewValue(tftypes.String, "cluster-1")
	vals["organization_name"] = tftypes.NewValue(tftypes.String, "acme")
	vals["plugin_name"] = tftypes.NewValue(tftypes.String, "grafana")
	vals["config"] = tftypes.NewValue(objType.AttributeTypes["config"], nil)
	vals["plugin_version"] = version
	return tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(objType, vals)}
}

func modifyCreatePlan(t *testing.T, client *FundamentClient, version tftypes.Value) resource.ModifyPlanResponse {
	t.Helper()
	s := pluginInstallationSchema(t)
	plan := createPlan(t, s, version)
	req := resource.ModifyPlanRequest{
		Plan:  plan,
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(context.Background()), nil)},
	}
	resp := resource.ModifyPlanResponse{Plan: plan}
	(&PluginInstallationResource{client: client}).ModifyPlan(context.Background(), req, &resp)
	return resp
}

func TestModifyPlan_UnpinnedWithoutInstallURLFailsAtPlan(t *testing.T) {
	resp := modifyCreatePlan(t, &FundamentClient{}, tftypes.NewValue(tftypes.String, tftypes.UnknownValue))
	require.True(t, resp.Diagnostics.HasError())
	assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "marketplace_install_url")
	assert.True(t, resp.Diagnostics.Errors()[0].(interface{ Path() path.Path }).Path().Equal(path.Root("plugin_version")))
}

func TestModifyPlan_PinnedWithoutInstallURLPasses(t *testing.T) {
	resp := modifyCreatePlan(t, &FundamentClient{}, tftypes.NewValue(tftypes.String, "1.2.3"))
	assert.False(t, resp.Diagnostics.HasError())
}

// The provider is not configured yet while its own config is unknown.
func TestModifyPlan_UnconfiguredClientPasses(t *testing.T) {
	resp := modifyCreatePlan(t, nil, tftypes.NewValue(tftypes.String, tftypes.UnknownValue))
	assert.False(t, resp.Diagnostics.HasError())
}
