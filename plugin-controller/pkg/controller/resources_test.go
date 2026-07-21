package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"

	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
	pluginmetadatav1 "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/metadata/proto/gen/v1"
)

func TestMutatePluginScopeClusterRole_MaterialisesRules(t *testing.T) {
	cr := &pluginsv1.PluginInstallation{}
	cr.Name = "cert-manager"

	rules := []*pluginmetadatav1.PolicyRule{
		{
			ApiGroups: []string{"cert-manager.io"},
			Resources: []string{"certificates"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			ApiGroups:     []string{""},
			Resources:     []string{"secrets"},
			Verbs:         []string{"get"},
			ResourceNames: []string{"cert-manager-webhook-ca"},
		},
	}

	role := &rbacv1.ClusterRole{}
	mutatePluginScopeClusterRole(role, cr, rules)

	require.Len(t, role.Rules, 2)
	assert.Equal(t, "cert-manager.io", role.Rules[0].APIGroups[0])
	assert.Equal(t, []string{"secrets"}, role.Rules[1].Resources)
	// resource_names must be threaded through: an empty ResourceNames grants
	// access to ALL objects of the resource, so a rule scoped to a named object
	// must stay scoped (regression guard for the dropped field).
	assert.Equal(t, []string{"cert-manager-webhook-ca"}, role.Rules[1].ResourceNames)
	assert.Empty(t, role.Rules[0].ResourceNames)
	assert.Equal(t, managedByValue, role.Labels[labelManagedBy])
}

func TestMutatePluginScopeClusterRoleBinding_BindsToPluginSA(t *testing.T) {
	cr := &pluginsv1.PluginInstallation{}
	cr.Name = "cert-manager"

	crb := &rbacv1.ClusterRoleBinding{}
	mutatePluginScopeClusterRoleBinding(crb, cr)

	assert.Equal(t, "ClusterRole", crb.RoleRef.Kind)
	assert.Equal(t, "plugin-cert-manager-scope", crb.RoleRef.Name)
	require.Len(t, crb.Subjects, 1)
	assert.Equal(t, rbacv1.ServiceAccountKind, crb.Subjects[0].Kind)
	assert.Equal(t, "plugin-cert-manager", crb.Subjects[0].Name)
	assert.Equal(t, "plugin-cert-manager", crb.Subjects[0].Namespace)
}

func TestPluginScopeNames(t *testing.T) {
	assert.Equal(t, "plugin-cert-manager-scope", pluginScopeClusterRoleName("cert-manager"))
}
