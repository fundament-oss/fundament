package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
)

func TestPluginInstallationResourceModel(t *testing.T) {
	model := PluginInstallationResourceModel{
		ID:             types.StringValue("cluster-123/grafana"),
		ClusterID:      types.StringValue("cluster-123"),
		PluginName:     types.StringValue("grafana"),
		PluginVersion:  types.StringValue("10.2.0"),
		DefinitionHash: types.StringValue("sha256:abc123"),
		Phase:          types.StringValue("Running"),
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
	if model.PluginVersion.ValueString() != "10.2.0" {
		t.Errorf("expected PluginVersion '10.2.0', got %q", model.PluginVersion.ValueString())
	}
	if model.DefinitionHash.ValueString() != "sha256:abc123" {
		t.Errorf("expected DefinitionHash 'sha256:abc123', got %q", model.DefinitionHash.ValueString())
	}
	if model.Phase.ValueString() != "Running" {
		t.Errorf("expected Phase 'Running', got %q", model.Phase.ValueString())
	}
}

func TestPluginInstallationCreatePayload_DefinitionRef(t *testing.T) {
	payload := pluginInstallationCreatePayload{
		APIVersion: pluginInstallationAPIVersion,
		Kind:       "PluginInstallation",
		Metadata:   pluginInstallationMetadata{Name: "grafana"},
		Spec: pluginInstallationSpec{
			DefinitionRef: pluginDefinitionRef{
				PluginName:     "grafana",
				PluginVersion:  "unknown",
				DefinitionHash: "sha256:unknown",
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	got := string(body)
	for _, want := range []string{
		`"definitionRef":{"pluginName":"grafana","pluginVersion":"unknown","definitionHash":"sha256:unknown"}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("payload %s does not contain %s", got, want)
		}
	}
	assert.NotContains(t, got, `"image"`, "payload must not contain image field")
	if strings.Contains(got, `"pluginName":"grafana","image"`) || strings.Contains(got, `"spec":{"pluginName"`) {
		t.Errorf("payload must not carry the legacy top-level spec.pluginName: %s", got)
	}
}

func TestPluginInstallationResourceModelNullValues(t *testing.T) {
	model := PluginInstallationResourceModel{
		ID:         types.StringNull(),
		ClusterID:  types.StringValue("cluster-123"),
		PluginName: types.StringValue("grafana"),
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
			cluster, plugin, err := parseImportID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("id %q: wantErr=%v, gotErr=%v", tt.id, tt.wantErr, err)
				return
			}
			if err == nil {
				if cluster != tt.wantCluster {
					t.Errorf("id %q: cluster = %q, want %q", tt.id, cluster, tt.wantCluster)
				}
				if plugin != tt.wantPlugin {
					t.Errorf("id %q: plugin = %q, want %q", tt.id, plugin, tt.wantPlugin)
				}
			}
		})
	}
}

func TestPluginInstallationResource_URLConstruction_Escaping(t *testing.T) {
	r := &PluginInstallationResource{
		client: &FundamentClient{KubeProxyURL: "https://proxy.example.com"},
	}

	got := r.resourceURL("c/d", "a b")
	want := "https://proxy.example.com/clusters/c%2Fd/apis/plugins.fundament.io/v1/plugininstallations/a%20b"
	if got != want {
		t.Errorf("resourceURL with special chars = %q, want %q", got, want)
	}
}

func TestClassifyPluginPhase(t *testing.T) {
	tests := []struct {
		phase        string
		wantDone     bool
		wantTerminal bool
	}{
		{phase: "Running", wantDone: true, wantTerminal: false},
		{phase: "Failed", wantDone: false, wantTerminal: true},
		{phase: "Terminating", wantDone: false, wantTerminal: true},
		{phase: "Degraded", wantDone: false, wantTerminal: true},
		{phase: "Pending", wantDone: false, wantTerminal: false},
		{phase: "Deploying", wantDone: false, wantTerminal: false},
		{phase: "", wantDone: false, wantTerminal: false},
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			done, terminal := classifyPluginPhase(tt.phase)
			if done != tt.wantDone || terminal != tt.wantTerminal {
				t.Errorf("classifyPluginPhase(%q) = (done=%v, terminal=%v), want (done=%v, terminal=%v)",
					tt.phase, done, terminal, tt.wantDone, tt.wantTerminal)
			}
		})
	}
}

func TestDefinitionHashRegex(t *testing.T) {
	hex64 := strings.Repeat("a", 64)

	valid := []string{
		"sha256:unknown",
		"sha256:" + hex64,
		"sha256:" + strings.Repeat("0", 64),
		"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
	for _, v := range valid {
		if !definitionHashRegex.MatchString(v) {
			t.Errorf("expected %q to be valid", v)
		}
	}

	invalid := []string{
		"",
		"unknown",
		"sha256:",
		"sha256:" + strings.Repeat("A", 64), // uppercase not allowed
		"sha256:" + strings.Repeat("a", 63), // too short
		"sha256:" + strings.Repeat("a", 65), // too long
		"sha512:" + hex64,                   // wrong algo prefix
		"sha256:xyz",                        // non-hex
		" sha256:unknown",                   // leading space
		"sha256:unknown ",                   // trailing space
	}
	for _, v := range invalid {
		if definitionHashRegex.MatchString(v) {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}
