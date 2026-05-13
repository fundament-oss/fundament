package v1

import (
	"encoding/json"
	"testing"
)

func TestPluginInstallationSpec_PermissionsRoundTrip(t *testing.T) {
	spec := PluginInstallationSpec{
		Image:      "ghcr.io/example/cert-manager:v1.0.0",
		PluginName: "cert-manager",
		Permissions: PluginPermissions{
			RBAC: []RBACRule{
				{
					APIGroups: []string{"cert-manager.io"},
					Resources: []string{"certificates", "certificaterequests"},
					Verbs:     []string{"get", "list", "watch"},
				},
			},
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

	if len(got.Permissions.RBAC) != 1 {
		t.Fatalf("RBAC = %v, want 1 rule", got.Permissions.RBAC)
	}
	rule := got.Permissions.RBAC[0]
	if rule.APIGroups[0] != "cert-manager.io" {
		t.Errorf("APIGroups = %v", rule.APIGroups)
	}
	if rule.Resources[1] != "certificaterequests" {
		t.Errorf("Resources = %v", rule.Resources)
	}
	if rule.Verbs[0] != "get" {
		t.Errorf("Verbs = %v", rule.Verbs)
	}
}

func TestPluginInstallationSpec_PermissionsOmittedWhenEmpty(t *testing.T) {
	spec := PluginInstallationSpec{
		Image:      "ghcr.io/example/cert-manager:v1.0.0",
		PluginName: "cert-manager",
	}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := string(data); contains(got, "permissions") {
		t.Errorf("expected 'permissions' to be omitted when empty, got JSON: %s", got)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
