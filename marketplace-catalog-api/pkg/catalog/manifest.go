package catalog

import (
	"fmt"
	"strings"

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
func configSchemaFromManifest(manifest []byte) ([]*catalogv1.ConfigSchemaEntry, error) {
	definition, err := pluginruntime.ParseDefinition(manifest)
	if err != nil {
		return nil, fmt.Errorf("parsing plugin definition: %w", err)
	}

	entries := make([]*catalogv1.ConfigSchemaEntry, 0, len(definition.Spec.ConfigSchema))
	for _, entry := range definition.Spec.ConfigSchema {
		entries = append(entries, catalogv1.ConfigSchemaEntry_builder{
			Name:         entry.Name,
			DisplayName:  entry.DisplayName,
			Type:         configTypeFromManifest(entry.Type),
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
func configTypeFromManifest(t string) catalogv1.ConfigType {
	switch t {
	case "string":
		return catalogv1.ConfigType_CONFIG_TYPE_STRING
	case "int":
		return catalogv1.ConfigType_CONFIG_TYPE_INT
	case "bool":
		return catalogv1.ConfigType_CONFIG_TYPE_BOOL
	case "enum":
		return catalogv1.ConfigType_CONFIG_TYPE_ENUM
	default:
		// Unreachable: parseDefinition already rejected unknown types.
		panic(fmt.Sprintf("unhandled config type %q", t))
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
