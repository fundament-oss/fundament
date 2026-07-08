package pluginruntime

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// PluginDefinition is the top-level plugin manifest, modeled after a
// Kubernetes resource with apiVersion, kind, metadata, and spec.
type PluginDefinition struct {
	APIVersion string         `yaml:"apiVersion"`
	Kind       string         `yaml:"kind"`
	Metadata   PluginMetadata `yaml:"metadata"`
	Spec       PluginSpec     `yaml:"spec"`
}

// PluginSpec contains the behavioral configuration of a plugin.
type PluginSpec struct {
	Permissions      Permissions                 `yaml:"permissions"`
	Menu             MenuDefinition              `yaml:"menu"`
	CustomComponents map[string]ComponentMapping `yaml:"customComponents"`
	UIHints          map[string]UIHint           `yaml:"uiHints"`
	CRDs             []string                    `yaml:"crds"`
	AllowedResources []AllowedResource           `yaml:"allowedResources"`
	// Image is the container image the plugin runs as, injected into the manifest
	// at publish time (never authored). Declaring it in the manifest — rather than
	// on the PluginInstallation CR — makes the manifest hash bind the exact code.
	// Always a digest reference (repo@sha256:...) in a published definition.
	Image string `yaml:"image"`
	// ImagePullPolicy mirrors corev1.PullPolicy ("Always"|"IfNotPresent"|"Never").
	ImagePullPolicy string `yaml:"imagePullPolicy"`
}

// AllowedResource declares a Kubernetes resource the plugin's UI iframe is
// permitted to read via the host-mediated SDK broker. The console enforces
// this allowlist before forwarding any iframe-initiated request to the
// kube-api-proxy.
type AllowedResource struct {
	Group    string   `yaml:"group"`
	Version  string   `yaml:"version"`
	Resource string   `yaml:"resource"`
	Verbs    []string `yaml:"verbs"`
}

// PluginMetadata holds the identifying information for a plugin.
type PluginMetadata struct {
	Name        string     `yaml:"name"`
	DisplayName string     `yaml:"displayName"`
	Version     string     `yaml:"version"`
	Description string     `yaml:"description"`
	Author      string     `yaml:"author"`
	License     string     `yaml:"license"`
	Icon        string     `yaml:"icon"`
	URLs        PluginURLs `yaml:"urls"`
	Tags        []string   `yaml:"tags"`
}

// PluginURLs holds links related to the plugin.
type PluginURLs struct {
	Homepage      string `yaml:"homepage"`
	Repository    string `yaml:"repository"`
	Documentation string `yaml:"documentation"`
}

// Permissions declares what a plugin needs from the platform.
type Permissions struct {
	Capabilities []string     `yaml:"capabilities"`
	RBAC         []PolicyRule `yaml:"rbac"`
}

// PolicyRule matches the Kubernetes RBAC PolicyRule structure.
type PolicyRule struct {
	APIGroups []string `yaml:"apiGroups"`
	Resources []string `yaml:"resources"`
	Verbs     []string `yaml:"verbs"`
	// ResourceNames optionally restricts the rule to named objects. Empty means
	// all objects of the resource — so a plugin scoping access to specific names
	// must set this, and it must survive the round-trip to the materialised
	// ClusterRole.
	ResourceNames []string `yaml:"resourceNames"`
}

// MenuDefinition describes how the plugin appears in the Fundament console.
type MenuDefinition struct {
	Organization []MenuEntry `yaml:"organization"`
	Project      []MenuEntry `yaml:"project"`
}

// MenuEntry maps a CRD to console UI pages.
type MenuEntry struct {
	CRD    string `yaml:"crd"`
	List   bool   `yaml:"list"`
	Detail bool   `yaml:"detail"`
	Icon   string `yaml:"icon"`
}

// ComponentMapping maps a CRD to custom UI component names.
type ComponentMapping struct {
	List   string `yaml:"list"`
	Detail string `yaml:"detail"`
	Create string `yaml:"create"`
}

// UIHint provides form layout and status display hints for a CRD.
type UIHint struct {
	FormGroups    []FormGroup   `yaml:"formGroups"`
	StatusMapping StatusMapping `yaml:"statusMapping"`
}

// FormGroup groups related fields in a create/edit form.
type FormGroup struct {
	Name   string   `yaml:"name"`
	Fields []string `yaml:"fields"`
}

// StatusMapping maps a JSON path to status badge display values.
type StatusMapping struct {
	JSONPath string                 `yaml:"jsonPath"`
	Values   map[string]StatusValue `yaml:"values"`
	Default  *StatusValue           `yaml:"default"`
}

// StatusValue describes how a status value is displayed.
type StatusValue struct {
	Badge string `yaml:"badge"`
	Label string `yaml:"label"`
}

// ParseDefinition decodes and validates a complete, published PluginDefinition
// from bytes. It is the shared parser used by organization-api and
// just plugin-publish. It is strict: a valid PluginDefinition always carries an
// image (the image-free source definition.yaml is a template, not a valid
// definition, until publish injects the image).
func ParseDefinition(data []byte) (PluginDefinition, error) {
	var def PluginDefinition
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&def); err != nil {
		return PluginDefinition{}, fmt.Errorf("parse plugin definition: %w", err)
	}
	if def.APIVersion != "fundament.io/v1" {
		return PluginDefinition{}, fmt.Errorf("unsupported apiVersion %q, expected \"fundament.io/v1\"", def.APIVersion)
	}
	if def.Kind != "PluginDefinition" {
		return PluginDefinition{}, fmt.Errorf("unsupported kind %q, expected \"PluginDefinition\"", def.Kind)
	}
	if def.Metadata.Name == "" {
		return PluginDefinition{}, fmt.Errorf("plugin definition is missing required field metadata.name")
	}
	if def.Spec.Image == "" {
		return PluginDefinition{}, fmt.Errorf("plugin definition is missing required field spec.image")
	}
	return def, nil
}

// PluginPhase represents the current lifecycle phase of a plugin.
type PluginPhase string

const (
	PhaseInstalling   PluginPhase = "installing"
	PhaseRunning      PluginPhase = "running"
	PhaseDegraded     PluginPhase = "degraded"
	PhaseFailed       PluginPhase = "failed"
	PhaseUninstalling PluginPhase = "uninstalling"
)

// PluginStatus represents the current status of a plugin.
type PluginStatus struct {
	Phase   PluginPhase
	Message string
}
