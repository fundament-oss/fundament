// Package definition models the parts of a published PluginDefinition that
// plugin-controller needs to materialise the plugin's ServiceAccount RBAC.
//
// A published PluginDefinition is immutable and content-addressed: it is
// resolved by its definition_hash (FUN-17 "Where the scope comes from"). The
// marketplace artifact store that serves definitions by hash is FUN-11 work;
// this package only models the consume side.
package definition

import "context"

// PluginDefinition is the (subset of the) plugin manifest the controller reads.
type PluginDefinition struct {
	PluginName    string
	PluginVersion string
	Permissions   Permissions
}

// Permissions mirrors the manifest's spec.permissions block.
type Permissions struct {
	RBAC []RBACRule
}

// RBACRule mirrors a Kubernetes rbac/v1 PolicyRule (subset). Subresources are
// expressed as "resource/subresource" entries in Resources.
type RBACRule struct {
	APIGroups     []string
	Resources     []string
	Verbs         []string
	ResourceNames []string
}

// Resolver resolves an immutable PluginDefinition by its content hash.
type Resolver interface {
	Resolve(ctx context.Context, definitionHash string) (*PluginDefinition, error)
}
