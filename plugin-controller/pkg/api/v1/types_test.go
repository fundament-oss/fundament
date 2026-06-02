package v1

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPluginInstallationSpec_DefinitionRefRoundTrip(t *testing.T) {
	spec := PluginInstallationSpec{
		Image: "ghcr.io/example/cert-manager:v1.17.2",
		DefinitionRef: DefinitionRef{
			PluginName:     "cert-manager",
			PluginVersion:  "v1.17.2",
			DefinitionHash: "sha256:1f3c9a",
		},
	}

	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got PluginInstallationSpec
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.DefinitionRef.PluginName != "cert-manager" {
		t.Errorf("DefinitionRef.PluginName = %q", got.DefinitionRef.PluginName)
	}
	if got.DefinitionRef.PluginVersion != "v1.17.2" {
		t.Errorf("DefinitionRef.PluginVersion = %q", got.DefinitionRef.PluginVersion)
	}
	if got.DefinitionRef.DefinitionHash != "sha256:1f3c9a" {
		t.Errorf("DefinitionRef.DefinitionHash = %q", got.DefinitionRef.DefinitionHash)
	}
}

func TestPluginInstallationSpec_DefinitionRefMarshalsExpectedKeys(t *testing.T) {
	spec := PluginInstallationSpec{
		Image: "ghcr.io/example/cert-manager:v1.17.2",
		DefinitionRef: DefinitionRef{
			PluginName:     "cert-manager",
			PluginVersion:  "v1.17.2",
			DefinitionHash: "sha256:1f3c9a",
		},
	}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := string(data); !strings.Contains(got, "definitionRef") {
		t.Errorf("expected JSON to contain 'definitionRef', got: %s", got)
	}
	if got := string(data); strings.Contains(got, "permissions") {
		t.Errorf("expected no 'permissions' key (removed by FUN-17), got: %s", got)
	}
}
