package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestPluginInstallationResourceModel(t *testing.T) {
	model := PluginInstallationResourceModel{
		ID:         types.StringValue("cluster-123/grafana"),
		ClusterID:  types.StringValue("cluster-123"),
		PluginName: types.StringValue("grafana"),
		Image:      types.StringValue("ghcr.io/fundament/grafana:v10.2.0"),
		Phase:      types.StringValue("Running"),
	}

	if model.ID.ValueString() != "cluster-123/grafana" {
		t.Errorf("expected ID 'cluster-123/grafana', got %q", model.ID.ValueString())
	}
	if model.ClusterID.ValueString() != "cluster-123" {
		t.Errorf("expected ClusterID 'cluster-123', got %q", model.ClusterID.ValueString())
	}
	if model.PluginName.ValueString() != "grafana" {
		t.Errorf("expected PluginName 'grafana', got %q", model.PluginName.ValueString())
	}
	if model.Image.ValueString() != "ghcr.io/fundament/grafana:v10.2.0" {
		t.Errorf("expected Image 'ghcr.io/fundament/grafana:v10.2.0', got %q", model.Image.ValueString())
	}
	if model.Phase.ValueString() != "Running" {
		t.Errorf("expected Phase 'Running', got %q", model.Phase.ValueString())
	}
}

func TestPluginInstallationResourceModelNullValues(t *testing.T) {
	model := PluginInstallationResourceModel{
		ID:         types.StringNull(),
		ClusterID:  types.StringValue("cluster-123"),
		PluginName: types.StringValue("grafana"),
		Image:      types.StringValue("ghcr.io/fundament/grafana:v10.2.0"),
		Phase:      types.StringNull(),
	}

	if !model.ID.IsNull() {
		t.Error("expected ID to be null")
	}
	if !model.Phase.IsNull() {
		t.Error("expected Phase to be null")
	}
	if model.ClusterID.IsNull() {
		t.Error("expected ClusterID to not be null")
	}
	if model.Image.IsNull() {
		t.Error("expected Image to not be null")
	}
}

func TestPluginInstallationResource_URLConstruction(t *testing.T) {
	r := &PluginInstallationResource{
		client: &FundamentClient{
			KubeProxyURL: "https://proxy.example.com",
		},
	}

	listURL := r.listURL("cluster-abc")
	expectedList := "https://proxy.example.com/clusters/cluster-abc/apis/plugins.fundament.io/v1/plugininstallations"
	if listURL != expectedList {
		t.Errorf("listURL = %q, want %q", listURL, expectedList)
	}

	resourceURL := r.resourceURL("cluster-abc", "grafana")
	expectedResource := expectedList + "/grafana"
	if resourceURL != expectedResource {
		t.Errorf("resourceURL = %q, want %q", resourceURL, expectedResource)
	}
}

func TestPluginInstallationResource_URLConstruction_TrailingSlash(t *testing.T) {
	r := &PluginInstallationResource{
		client: &FundamentClient{
			KubeProxyURL: "https://proxy.example.com/",
		},
	}

	listURL := r.listURL("cluster-abc")
	expected := "https://proxy.example.com/clusters/cluster-abc/apis/plugins.fundament.io/v1/plugininstallations"
	if listURL != expected {
		t.Errorf("listURL with trailing slash = %q, want %q", listURL, expected)
	}
}

func TestPluginInstallationResource_ImportID_parsing(t *testing.T) {
	tests := []struct {
		id          string
		wantCluster string
		wantPlugin  string
		wantErr     bool
	}{
		{
			id:          "cluster-abc/grafana",
			wantCluster: "cluster-abc",
			wantPlugin:  "grafana",
		},
		{
			id:          "some-uuid/my-plugin",
			wantCluster: "some-uuid",
			wantPlugin:  "my-plugin",
		},
		{
			id:          "cluster/plugin/extra",
			wantCluster: "cluster",
			wantPlugin:  "plugin/extra",
		},
		{id: "no-slash", wantErr: true},
		{id: "/no-cluster", wantErr: true},
		{id: "no-plugin/", wantErr: true},
		{id: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			parts := strings.SplitN(tt.id, "/", 2)
			invalid := len(parts) != 2 || parts[0] == "" || parts[1] == ""
			if invalid != tt.wantErr {
				t.Errorf("id %q: wantErr=%v, gotErr=%v", tt.id, tt.wantErr, invalid)
				return
			}
			if !invalid {
				if parts[0] != tt.wantCluster {
					t.Errorf("id %q: cluster = %q, want %q", tt.id, parts[0], tt.wantCluster)
				}
				if parts[1] != tt.wantPlugin {
					t.Errorf("id %q: plugin = %q, want %q", tt.id, parts[1], tt.wantPlugin)
				}
			}
		})
	}
}
