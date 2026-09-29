package pluginruntime

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// HashManifest returns the content hash that pins a PluginDefinition: the sha256
// of the raw manifest bytes, prefixed with "sha256:". organization-api derives
// this when storing a definition and the plugin-controller derives it when
// verifying an install's consent pin — they MUST agree byte-for-byte, so this is
// the single shared implementation both call.
func HashManifest(manifest []byte) string {
	sum := sha256.Sum256(manifest)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// imageDigestRefRegex requires a digest-pinned image reference
// (repo@sha256:<64 hex chars>). A published PluginDefinition must pin an
// immutable digest, never a mutable tag, so the manifest hash binds the exact
// code that runs. A sha256 digest is exactly 64 hex characters — anchoring the
// length rejects a truncated or malformed digest that `+` would wave through.
var imageDigestRefRegex = regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)

// validImagePullPolicies is the set ParseDefinition accepts for
// spec.imagePullPolicy. Empty is allowed (Kubernetes applies its default); any
// other value would be rejected by the plugin cluster's apiserver when the
// Deployment is created, so reject it at parse/publish time instead.
var validImagePullPolicies = map[string]bool{"": true, "Always": true, "IfNotPresent": true, "Never": true}

// configKeyNameRegex constrains configSchema key names to what survives the
// FUNP_<NAME> env-var injection unmangled: uppercase, digits, underscores,
// starting with a letter.
var configKeyNameRegex = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// The types a configSchema entry may declare, as the manifest spells them.
// This is the one authoritative vocabulary: validation here, the catalog's
// proto-enum mapping, and any other Go consumer key off these constants (the
// console keys off the catalog's generated enum).
const (
	ConfigTypeString = "string"
	ConfigTypeInt    = "int"
	ConfigTypeBool   = "bool"
	ConfigTypeEnum   = "enum"
)

// The placeholder spec.definitionRef pin written by installers (terraform,
// console) until the marketplace supplies real versions and hashes (FUN-11).
// Spelled once here for every Go consumer; the console mirrors them in TS
// (UNKNOWN_PLUGIN_VERSION in plugin-installation.service.ts).
const (
	// UnknownVersion resolves no stored definition — the version is the fetch
	// key, so an unpinned version has nothing to fetch.
	UnknownVersion = "unknown"
	// UnknownHash can never equal a real digest, so it is treated as unpinned
	// rather than verified (verifying would reject every install forever).
	UnknownHash = "sha256:unknown"
)

// validConfigTypes is the set of types a configSchema entry may declare.
// Values stay strings in spec.config; the type drives validation and the
// console's input widget, not the representation.
var validConfigTypes = map[string]bool{
	ConfigTypeString: true,
	ConfigTypeInt:    true,
	ConfigTypeBool:   true,
	ConfigTypeEnum:   true,
}

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
	// ConfigSchema declares the install-time config keys the plugin accepts.
	// The console renders its install form from this list (in order) and
	// plugin-controller rejects a spec.config that violates it. An empty list
	// means the plugin declares no schema and any config is accepted.
	ConfigSchema []ConfigSchemaEntry `yaml:"configSchema"`
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

// ConfigSchemaEntry declares one install-time config key. Default and Values
// are strings because PluginInstallation.spec.config is map[string]string;
// typing lives in validation and the form, not the representation.
type ConfigSchemaEntry struct {
	// Name is the key in spec.config, injected as the FUNP_<Name> env var.
	Name string `yaml:"name"`
	// DisplayName is the human-readable label the console shows for this key;
	// optional. Empty means the console derives a label from Name.
	DisplayName string `yaml:"displayName"`
	// Type is one of "string", "int", "bool", "enum".
	Type        string `yaml:"type"`
	Default     string `yaml:"default"`
	Description string `yaml:"description"`
	// Required forces the admin to choose a value; mutually exclusive with
	// Default, which would make the choice silently optional.
	Required bool `yaml:"required"`
	// Values enumerates the allowed values; set exactly when Type is "enum".
	Values []string `yaml:"values"`
	// Advanced keys start collapsed in the console install form.
	Advanced bool `yaml:"advanced"`
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
	// Label overrides the sidebar entry text. When empty the console derives a
	// label from the CRD name, which mangles the full "<plural>.<group>" form.
	Label string `yaml:"label"`
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
// just plugins publish. It is strict: a valid PluginDefinition always carries an
// image (the image-free source definition.yaml is a template, not a valid
// definition, until publish injects the image).
func ParseDefinition(data []byte) (PluginDefinition, error) {
	return parseDefinition(data, true)
}

// ParseSourceDefinition decodes and validates an *authored* definition.yaml: the
// image-free source manifest a plugin author edits and commits, before publish
// injects the pushed image digest. It applies every check ParseDefinition does
// except requiring spec.image to be present. An image that IS present is still
// required to be a digest reference, so a hand-pinned mutable tag is rejected
// here rather than at publish time.
//
// Use ParseDefinition for anything that consumes a published manifest: the digest
// pin is what binds the manifest hash to the exact code that runs, so dropping
// that requirement is only ever correct for a manifest that has not been
// published yet.
func ParseSourceDefinition(data []byte) (PluginDefinition, error) {
	return parseDefinition(data, false)
}

func parseDefinition(data []byte, requireImage bool) (PluginDefinition, error) {
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
	if def.Spec.Image == "" && requireImage {
		return PluginDefinition{}, fmt.Errorf("plugin definition is missing required field spec.image")
	}
	if def.Spec.Image != "" && !imageDigestRefRegex.MatchString(def.Spec.Image) {
		return PluginDefinition{}, fmt.Errorf("spec.image %q must be a digest reference (repo@sha256:<64 hex chars>), not a mutable tag", def.Spec.Image)
	}
	if !validImagePullPolicies[def.Spec.ImagePullPolicy] {
		return PluginDefinition{}, fmt.Errorf("spec.imagePullPolicy %q is invalid, expected one of \"Always\", \"IfNotPresent\", \"Never\" (or empty)", def.Spec.ImagePullPolicy)
	}
	if err := validateConfigSchema(def.Spec.ConfigSchema); err != nil {
		return PluginDefinition{}, err
	}
	return def, nil
}

// validateConfigSchema enforces the publish-time rules on a declared schema.
// It runs inside parseDefinition, so every publish path (marketplace-registry,
// legacy organization-api, functl) rejects a malformed schema identically.
func validateConfigSchema(schema []ConfigSchemaEntry) error {
	seen := make(map[string]bool, len(schema))
	for _, entry := range schema {
		if !configKeyNameRegex.MatchString(entry.Name) {
			return fmt.Errorf("configSchema key %q must match %s (it becomes the FUNP_ env var name)", entry.Name, configKeyNameRegex)
		}
		if seen[entry.Name] {
			return fmt.Errorf("configSchema declares key %q twice", entry.Name)
		}
		seen[entry.Name] = true
		if !validConfigTypes[entry.Type] {
			return fmt.Errorf("configSchema key %q has invalid type %q, expected one of \"string\", \"int\", \"bool\", \"enum\"", entry.Name, entry.Type)
		}
		if entry.Type == ConfigTypeEnum && len(entry.Values) == 0 {
			return fmt.Errorf("configSchema key %q is an enum but declares no values", entry.Name)
		}
		// A required value must be non-blank at install time, so a required
		// enum whose values are all blank could never be satisfied by any
		// config — reject the deadlock at publish time.
		if entry.Type == ConfigTypeEnum && entry.Required && !slices.ContainsFunc(entry.Values, func(v string) bool { return strings.TrimSpace(v) != "" }) {
			return fmt.Errorf("configSchema key %q is a required enum but declares no non-blank value", entry.Name)
		}
		if entry.Type != ConfigTypeEnum && len(entry.Values) > 0 {
			return fmt.Errorf("configSchema key %q declares values but is not an enum", entry.Name)
		}
		if entry.Required && entry.Default != "" {
			return fmt.Errorf("configSchema key %q sets both required and a default; they are mutually exclusive (required means the admin must choose)", entry.Name)
		}
		if entry.Required && entry.Advanced {
			return fmt.Errorf("configSchema key %q is required and advanced; a required choice cannot default to hidden", entry.Name)
		}
		if entry.Type == ConfigTypeBool {
			if entry.Required {
				return fmt.Errorf("configSchema key %q is a required bool; booleans cannot be required (declare a default instead)", entry.Name)
			}
			if entry.Default == "" {
				return fmt.Errorf("configSchema key %q is a bool without a default; a checkbox has no absent state, so declare the default explicitly", entry.Name)
			}
		}
		if entry.Default != "" {
			if err := validateConfigValue(entry.Default, entry); err != nil {
				return fmt.Errorf("configSchema key %q default: %w", entry.Name, err)
			}
			// The console form compares defaults with a strict 'true'/'false'
			// spelling; constrain the contract here rather than loosen the
			// client to accept every strconv.ParseBool spelling ("True", "1"...).
			if entry.Type == ConfigTypeBool && entry.Default != "true" && entry.Default != "false" {
				return fmt.Errorf("configSchema key %q bool default must be exactly \"true\" or \"false\", got %q", entry.Name, entry.Default)
			}
		}
	}
	return nil
}

// validateConfigValue checks one value against its declared entry. Bool uses
// strconv.ParseBool to match what caarlos0/env accepts in the plugin binary,
// so a value the schema passes never fails inside the pod.
func validateConfigValue(value string, entry ConfigSchemaEntry) error {
	switch entry.Type {
	case ConfigTypeString:
		return nil
	case ConfigTypeInt:
		// No explicit '+': strconv.ParseInt would accept it, but the console
		// form's int pattern (a single shared grammar) does not — a value the
		// server admits must be re-enterable through the form.
		if strings.HasPrefix(value, "+") {
			return fmt.Errorf("%q must not carry an explicit '+'", value)
		}
		if _, err := strconv.ParseInt(value, 10, 64); err != nil {
			return fmt.Errorf("%q is not an integer", value)
		}
		return nil
	case ConfigTypeBool:
		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("%q is not a boolean", value)
		}
		return nil
	case ConfigTypeEnum:
		if slices.Contains(entry.Values, value) {
			return nil
		}
		return fmt.Errorf("%q is not one of %v", value, entry.Values)
	default:
		// Unreachable: validateConfigSchema rejects unknown types before any
		// value is checked against them.
		panic(fmt.Sprintf("unhandled config type %q", entry.Type))
	}
}

// sdkReservedConfigKeys are the config keys every plugin binary reads through
// the SDK itself (Config in config.go: FUNP_LOG_LEVEL, FUNP_RECONCILE_INTERVAL),
// independent of what its definition declares.
// A schema-bearing definition accepts them without declaring them — otherwise
// declaring any schema would break e.g. LOG_LEVEL on every install. Values are
// checked with the same parsers the SDK's env decoding applies, so a value
// admitted here never fails inside the pod. A schema MAY declare one of these
// keys itself (e.g. to constrain it to an enum); the declared entry then wins.
var sdkReservedConfigKeys = map[string]func(value string) error{
	"LOG_LEVEL": func(value string) error {
		var level slog.Level
		if err := level.UnmarshalText([]byte(value)); err != nil {
			return fmt.Errorf("%q is not a log level", value)
		}
		return nil
	},
	"RECONCILE_INTERVAL": func(value string) error {
		if _, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("%q is not a duration", value)
		}
		return nil
	},
}

// ValidateConfig checks a PluginInstallation's spec.config against the
// definition's declared configSchema. An empty schema accepts anything —
// definitions published before configSchema existed keep their historical
// accept-anything behavior. A declared schema rejects undeclared keys (except
// the SDK-reserved ones every plugin reads), values that fail their declared
// type, and missing required keys. Shared between publish-side tooling and
// plugin-controller so the two can never drift.
func ValidateConfig(config map[string]string, schema []ConfigSchemaEntry) error {
	if len(schema) == 0 {
		return nil
	}
	entries := make(map[string]ConfigSchemaEntry, len(schema))
	for _, entry := range schema {
		entries[entry.Name] = entry
	}
	// Sorted so a multi-error config reports the same first error every
	// reconcile, keeping the status condition stable.
	for _, key := range slices.Sorted(maps.Keys(config)) {
		entry, declared := entries[key]
		if !declared {
			if validateReserved, reserved := sdkReservedConfigKeys[key]; reserved {
				if err := validateReserved(config[key]); err != nil {
					return fmt.Errorf("config key %q: %w", key, err)
				}
				continue
			}
			return fmt.Errorf("config key %q is not declared in the definition's configSchema", key)
		}
		if err := validateConfigValue(config[key], entry); err != nil {
			return fmt.Errorf("config key %q: %w", key, err)
		}
	}
	for _, entry := range schema {
		if entry.Required {
			value, set := config[entry.Name]
			if !set {
				return fmt.Errorf("config key %q is required by the definition's configSchema but not set", entry.Name)
			}
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("config key %q is required by the definition's configSchema but empty", entry.Name)
			}
		}
	}
	return nil
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
