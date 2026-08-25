package catalog

import (
	"fmt"
	"strings"

	marketplacev1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/marketplace/v1"
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

// Collapses RBAC verbs into the two phrases the storefront shows. bind,
// escalate and impersonate hand out privileges, so they read as write too.
func accessLabel(verbs []string) string {
	for _, verb := range verbs {
		switch verb {
		case "create", "update", "patch", "delete", "deletecollection",
			"bind", "escalate", "impersonate", "*":
			return "Read and write"
		}
	}
	return "Read"
}
