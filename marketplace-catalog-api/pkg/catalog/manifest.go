package catalog

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	marketplacev1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/marketplace/v1"
	catalogv1 "github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/proto/gen/catalog/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

// Derived from the pinned manifest rather than stored in columns, so they can
// never drift from the hash the install pins against.
func capabilitiesAndPermissions(manifest []byte) ([]string, []*marketplacev1.PluginPermission, error) {
	definition, err := pluginruntime.ParseDefinition(manifest)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing plugin definition: %w", err)
	}

	capabilities := definition.Spec.Permissions.Capabilities

	permissions := make([]*marketplacev1.PluginPermission, 0, len(definition.Spec.Permissions.RBAC))
	for _, rule := range definition.Spec.Permissions.RBAC {
		permissions = append(permissions, marketplacev1.PluginPermission_builder{
			Resource: strings.Join(rule.Resources, ", "),
			Access:   accessLabel(rule.Verbs),
		}.Build())
	}

	return capabilities, permissions, nil
}

// configSchemaFromManifest derives the install-form schema from the pinned
// manifest, like capabilitiesAndPermissions above: never stored in a column,
// so it can never drift from the hash.
//
// Unlike the publish-time parser, this decode deliberately tolerates unknown
// fields: manifests outlive the binary that parses them, and a manifest
// published by a newer plugin-sdk must not lose its install form — the
// console fails closed on "schema unavailable", which would block every
// install of that version, config or not, until the catalog is upgraded.
// Only a configSchema this binary cannot faithfully project (undecodable
// yaml, a config type it does not know) reports unavailable.
func configSchemaFromManifest(manifest []byte) ([]*catalogv1.ConfigSchemaEntry, error) {
	var definition struct {
		Spec struct {
			ConfigSchema []pluginruntime.ConfigSchemaEntry `yaml:"configSchema"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(manifest, &definition); err != nil {
		return nil, fmt.Errorf("parsing plugin definition: %w", err)
	}

	entries := make([]*catalogv1.ConfigSchemaEntry, 0, len(definition.Spec.ConfigSchema))
	for _, entry := range definition.Spec.ConfigSchema {
		configType, err := configTypeFromManifest(entry.Type)
		if err != nil {
			return nil, err
		}
		entries = append(entries, catalogv1.ConfigSchemaEntry_builder{
			Name:         entry.Name,
			DisplayName:  entry.DisplayName,
			Type:         configType,
			DefaultValue: entry.Default,
			Description:  entry.Description,
			Required:     entry.Required,
			Values:       entry.Values,
			Advanced:     entry.Advanced,
		}.Build())
	}
	return entries, nil
}

// configTypeFromManifest maps the manifest's type string to the proto enum.
func configTypeFromManifest(t string) (catalogv1.ConfigType, error) {
	switch t {
	case "string":
		return catalogv1.ConfigType_CONFIG_TYPE_STRING, nil
	case "int":
		return catalogv1.ConfigType_CONFIG_TYPE_INT, nil
	case "bool":
		return catalogv1.ConfigType_CONFIG_TYPE_BOOL, nil
	case "enum":
		return catalogv1.ConfigType_CONFIG_TYPE_ENUM, nil
	default:
		// Reachable under the lenient decode above: a newer SDK may declare
		// types this binary does not know, and a form it cannot render
		// correctly must report the schema unavailable instead.
		return 0, fmt.Errorf("config type %q unknown to this catalog", t)
	}
}

// Collapses RBAC verbs into the two phrases the storefront shows. bind,
// escalate and impersonate hand out privileges, so they read as write too.
func accessLabel(verbs []string) string {
	for _, verb := range verbs {
		switch verb {
		case "create", "update", "patch", "delete", "deletecollection",
			"bind", "escalate", "impersonate", "approve", "sign", "use", "*":
			return "Read and write"
		}
	}
	return "Read"
}
