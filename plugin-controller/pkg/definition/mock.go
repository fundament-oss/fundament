package definition

import (
	"context"
	"fmt"
)

// mockResolver serves a fixed set of definitions for dev and tests. The real
// resolver (marketplace artifact store, FUN-11) is wired separately.
type mockResolver struct {
	defs map[string]*PluginDefinition
}

// NewMockResolver returns a Resolver that knows the canned "sha256:mock"
// cert-manager definition used by mock-mode plugin-proxy (Plan B).
func NewMockResolver() Resolver {
	return &mockResolver{
		defs: map[string]*PluginDefinition{
			"sha256:mock": {
				PluginName:    "cert-manager",
				PluginVersion: "v1.17.2",
				Permissions: Permissions{
					RBAC: []RBACRule{
						{
							APIGroups: []string{"cert-manager.io"},
							Resources: []string{"certificates", "certificaterequests"},
							Verbs:     []string{"get", "list", "watch"},
						},
						{
							APIGroups:     []string{""},
							Resources:     []string{"secrets"},
							Verbs:         []string{"get"},
							ResourceNames: []string{"cert-manager-webhook-ca"},
						},
					},
				},
			},
		},
	}
}

func (m *mockResolver) Resolve(_ context.Context, definitionHash string) (*PluginDefinition, error) {
	def, ok := m.defs[definitionHash]
	if !ok {
		return nil, fmt.Errorf("definition %q not found", definitionHash)
	}
	return def, nil
}
