package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

func TestMutatePluginScopeClusterRole_MaterialisesRules(t *testing.T) {
	cr := &pluginsv1.PluginInstallation{}
	cr.Name = "cert-manager"

	rules := []pluginruntime.PolicyRule{
		{
			APIGroups: []string{"cert-manager.io"},
			Resources: []string{"certificates"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups:     []string{""},
			Resources:     []string{"secrets"},
			Verbs:         []string{"get"},
			ResourceNames: []string{"cert-manager-ca"},
		},
	}

	role := &rbacv1.ClusterRole{}
	mutatePluginScopeClusterRole(role, cr, rules)

	require.Len(t, role.Rules, 2)
	assert.Equal(t, "cert-manager.io", role.Rules[0].APIGroups[0])
	assert.Equal(t, []string{"secrets"}, role.Rules[1].Resources)
	// resourceNames declared in the manifest must scope the materialised rule to
	// named objects — otherwise the plugin SA gets broader RBAC than declared.
	assert.Equal(t, []string{"cert-manager-ca"}, role.Rules[1].ResourceNames)
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

func TestMutateDeployment_UsesManifestImage(t *testing.T) {
	cr := &pluginsv1.PluginInstallation{}
	cr.Name = "cert-manager"
	cr.Spec.Config = map[string]string{"LOG_LEVEL": "debug"}

	def := pluginruntime.PluginDefinition{
		Spec: pluginruntime.PluginSpec{
			Image:           "quay.io/jetstack/cert-manager-controller@sha256:deadbeef",
			ImagePullPolicy: "IfNotPresent",
		},
	}
	envVars := []corev1.EnvVar{
		{Name: "FUNDAMENT_CLUSTER_ID", Value: "test-cluster"},
	}
	deploy := &appsv1.Deployment{}
	mutateDeployment(deploy, cr, &def, envVars)

	require.Len(t, deploy.Spec.Template.Spec.Containers, 1)
	container := deploy.Spec.Template.Spec.Containers[0]
	assert.Equal(t, "quay.io/jetstack/cert-manager-controller@sha256:deadbeef", container.Image)
	assert.Equal(t, corev1.PullIfNotPresent, container.ImagePullPolicy)
	assert.Equal(t, "cert-manager", container.Name)

	// Should have fundament env vars + config env vars
	foundClusterID := false
	foundLogLevel := false
	for _, env := range container.Env {
		if env.Name == "FUNDAMENT_CLUSTER_ID" {
			foundClusterID = true
			assert.Equal(t, "test-cluster", env.Value)
		}
		if env.Name == "FUNP_LOG_LEVEL" {
			foundLogLevel = true
			assert.Equal(t, "debug", env.Value)
		}
	}
	assert.True(t, foundClusterID, "FUNDAMENT_CLUSTER_ID env var should be present")
	assert.True(t, foundLogLevel, "LOG_LEVEL env var should be present")

	// Health probes
	assert.NotNil(t, container.LivenessProbe)
	assert.NotNil(t, container.ReadinessProbe)
	assert.Equal(t, "/livez", container.LivenessProbe.HTTPGet.Path)
	assert.Equal(t, "/readyz", container.ReadinessProbe.HTTPGet.Path)
}
