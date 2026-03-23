package pluginruntime

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// PluginDefinition contains the static metadata for a plugin.
type PluginDefinition struct {
	Metadata         PluginMetadata              `yaml:"metadata"`
	Permissions      Permissions                 `yaml:"permissions"`
	Menu             MenuDefinition              `yaml:"menu"`
	CustomComponents map[string]ComponentMapping `yaml:"customComponents"`
	UIHints          map[string]UIHint           `yaml:"uiHints"`
	CRDs             []string                    `yaml:"crds"`
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
	Create bool   `yaml:"create"`
	Icon   string `yaml:"icon"`
}

// ComponentMapping maps a CRD to custom UI component names.
type ComponentMapping struct {
	List   string `yaml:"list"`
	Detail string `yaml:"detail"`
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
}

// StatusValue describes how a status value is displayed.
type StatusValue struct {
	Badge string `yaml:"badge"`
	Label string `yaml:"label"`
}

// yamlManifest is the top-level YAML structure including apiVersion and kind.
type yamlManifest struct {
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	Spec       PluginDefinition `yaml:"spec"`
}

// LoadDefinition reads a YAML plugin manifest from path and returns
// the PluginDefinition. It validates that apiVersion is "fundament.io/v1"
// and kind is "PluginDefinition".
func LoadDefinition(path string) (PluginDefinition, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is provided by plugin developer, not user input
	if err != nil {
		return PluginDefinition{}, fmt.Errorf("read plugin definition: %w", err)
	}

	var manifest yamlManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return PluginDefinition{}, fmt.Errorf("parse plugin definition: %w", err)
	}

	if manifest.APIVersion != "fundament.io/v1" {
		return PluginDefinition{}, fmt.Errorf("unsupported apiVersion %q, expected \"fundament.io/v1\"", manifest.APIVersion)
	}
	if manifest.Kind != "PluginDefinition" {
		return PluginDefinition{}, fmt.Errorf("unsupported kind %q, expected \"PluginDefinition\"", manifest.Kind)
	}

	return manifest.Spec, nil
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
