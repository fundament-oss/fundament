package controller

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	psaapi "k8s.io/pod-security-admission/api"
	"k8s.io/pod-security-admission/policy"

	"github.com/fundament-oss/fundament/common/kubename"
	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

func newInstallation(name, org, plugin string) *pluginsv1.PluginInstallation {
	cr := &pluginsv1.PluginInstallation{}
	cr.Name = name
	cr.Spec.DefinitionRef.OrganizationName = org
	cr.Spec.DefinitionRef.PluginName = plugin
	return cr
}

func TestValidateInstallation_AcceptsQualifiedName(t *testing.T) {
	require.NoError(t, validateInstallation(newInstallation("acme--cert-manager", "acme", "cert-manager")))
}

func TestValidateInstallation_RejectsMismatchedName(t *testing.T) {
	err := validateInstallation(newInstallation("cert-manager", "acme", "cert-manager"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "acme--cert-manager")
}

func TestValidateInstallation_RejectsMissingOrganization(t *testing.T) {
	err := validateInstallation(newInstallation("cert-manager", "", "cert-manager"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "organizationName")
}

func TestValidateInstallation_RejectsMissingPluginName(t *testing.T) {
	err := validateInstallation(newInstallation("acme--", "acme", ""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pluginName")
}

func TestValidateInstallation_RejectsInvalidCharacters(t *testing.T) {
	// A dot is legal in a DNS subdomain but not in the namespace we derive.
	err := validateInstallation(newInstallation("acme--cert.manager", "acme", "cert.manager"))
	require.Error(t, err)
}

func TestValidateInstallation_RejectsOverLongName(t *testing.T) {
	plugin := strings.Repeat("p", 240)
	err := validateInstallation(newInstallation("acme--"+plugin, "acme", plugin))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "239")
}

func TestChildResourcesUseConstantNames(t *testing.T) {
	cr := newInstallation("acme--cert-manager", "acme", "cert-manager")

	crb := &rbacv1.ClusterRoleBinding{}
	mutatePluginScopeClusterRoleBinding(crb, cr)

	require.Len(t, crb.Subjects, 1)
	// The namespace disambiguates, so the SA needs no name-derived suffix.
	assert.Equal(t, "plugin", crb.Subjects[0].Name)
	assert.Equal(t, "plugin-acme--cert-manager", crb.Subjects[0].Namespace)
	assert.Equal(t, "plugin-acme--cert-manager-scope", crb.RoleRef.Name)
}

func TestMutateDeployment_UsesConstantContainerName(t *testing.T) {
	cr := newInstallation("acme--cert-manager", "acme", "cert-manager")
	def := pluginruntime.PluginDefinition{
		Spec: pluginruntime.PluginSpec{Image: "repo@sha256:" + strings.Repeat("a", 64)},
	}

	deploy := &appsv1.Deployment{}
	mutateDeployment(deploy, cr, &def, nil, "")

	require.Len(t, deploy.Spec.Template.Spec.Containers, 1)
	// A container name is a 63-char DNS label; the installation name is not.
	assert.Equal(t, "plugin", deploy.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, "plugin", deploy.Spec.Template.Spec.ServiceAccountName)
}

func TestSelectorLabels_FitLabelValueLimit(t *testing.T) {
	long := strings.Repeat("x", 200)
	cr := newInstallation("acme--"+long, "acme", long)

	for k, v := range selectorLabels(cr) {
		assert.LessOrEqual(t, len(v), 63, "label %q value must fit the 63-char limit", k)
	}
}

func TestPluginNamespace_MatchesSharedHelper(t *testing.T) {
	assert.Equal(t, kubename.PluginNamespace("acme--cert-manager"), pluginNamespace("acme--cert-manager"))
}

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
	assert.Equal(t, "plugin", crb.Subjects[0].Name)
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
			Image:           "quay.io/jetstack/cert-manager-controller@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
			ImagePullPolicy: "IfNotPresent",
		},
	}
	envVars := []corev1.EnvVar{
		{Name: "FUNDAMENT_CLUSTER_ID", Value: "test-cluster"},
	}
	deploy := &appsv1.Deployment{}
	mutateDeployment(deploy, cr, &def, envVars, "")

	require.Len(t, deploy.Spec.Template.Spec.Containers, 1)
	container := deploy.Spec.Template.Spec.Containers[0]
	assert.Equal(t, "quay.io/jetstack/cert-manager-controller@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef", container.Image)
	assert.Equal(t, corev1.PullIfNotPresent, container.ImagePullPolicy)
	// The namespace disambiguates, so the container needs no name-derived suffix.
	assert.Equal(t, "plugin", container.Name)

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

func TestMutateDeployment_MeetsRestrictedPodSecurity(t *testing.T) {
	cr := newInstallation("acme--cert-manager", "acme", "cert-manager")
	def := pluginruntime.PluginDefinition{
		Spec: pluginruntime.PluginSpec{Image: "repo@sha256:" + strings.Repeat("a", 64)},
	}

	deploy := &appsv1.Deployment{}
	mutateDeployment(deploy, cr, &def, nil, "")

	evaluator, err := policy.NewEvaluator(policy.DefaultChecks(), nil)
	require.NoError(t, err)
	lv := psaapi.LevelVersion{Level: psaapi.LevelRestricted, Version: psaapi.LatestVersion()}
	for _, res := range evaluator.EvaluatePod(lv, &deploy.Spec.Template.ObjectMeta, &deploy.Spec.Template.Spec) {
		assert.True(t, res.Allowed, "%s: %s", res.ForbiddenReason, res.ForbiddenDetail)
	}
}

func TestMutateDeployment_GivesHelmAWritableHome(t *testing.T) {
	cr := newInstallation("acme--cert-manager", "acme", "cert-manager")
	def := pluginruntime.PluginDefinition{
		Spec: pluginruntime.PluginSpec{Image: "repo@sha256:" + strings.Repeat("a", 64)},
	}

	deploy := &appsv1.Deployment{}
	mutateDeployment(deploy, cr, &def, nil, "")

	container := deploy.Spec.Template.Spec.Containers[0]
	// A non-root uid without a passwd entry gets HOME=/, where helm cannot
	// create its cache; HOME must point at the writable emptyDir.
	assert.Contains(t, container.Env, corev1.EnvVar{Name: "HOME", Value: "/tmp"})
	// The emptyDir is the only writable path: the image filesystem is read-only.
	require.NotNil(t, container.SecurityContext)
	assert.Equal(t, new(true), container.SecurityContext.ReadOnlyRootFilesystem)
	assert.Contains(t, container.VolumeMounts, corev1.VolumeMount{Name: "tmp", MountPath: "/tmp"})
	require.Len(t, deploy.Spec.Template.Spec.Volumes, 1)
	assert.Equal(t, "tmp", deploy.Spec.Template.Spec.Volumes[0].Name)
	assert.NotNil(t, deploy.Spec.Template.Spec.Volumes[0].EmptyDir)
}

func TestMutateDeployment_DevImageOverridesManifestImage(t *testing.T) {
	cr := newInstallation("acme--cert-manager", "acme", "cert-manager")
	def := pluginruntime.PluginDefinition{
		Spec: pluginruntime.PluginSpec{
			Image:           "quay.io/jetstack/cert-manager-controller@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
			ImagePullPolicy: "IfNotPresent",
		},
	}
	devImage := "localhost:5112/foo-plugin-dev:abc@sha256:" + strings.Repeat("a", 64)

	deploy := &appsv1.Deployment{}
	mutateDeployment(deploy, cr, &def, nil, devImage)

	require.Len(t, deploy.Spec.Template.Spec.Containers, 1)
	container := deploy.Spec.Template.Spec.Containers[0]
	// Byte-for-byte: skaffold file sync matches pods by string equality
	// between container.Image and the built artifact tag.
	assert.Equal(t, devImage, container.Image)
	assert.Equal(t, corev1.PullIfNotPresent, container.ImagePullPolicy)
}

func TestMutateDeployment_DevModeRelaxations(t *testing.T) {
	cr := newInstallation("acme--cert-manager", "acme", "cert-manager")
	def := pluginruntime.PluginDefinition{
		Spec: pluginruntime.PluginSpec{Image: "repo@sha256:" + strings.Repeat("a", 64)},
	}
	devImage := "localhost:5112/foo-plugin-dev:abc@sha256:" + strings.Repeat("a", 64)

	devDeploy := &appsv1.Deployment{}
	mutateDeployment(devDeploy, cr, &def, nil, devImage)
	devContainer := devDeploy.Spec.Template.Spec.Containers[0]
	require.NotNil(t, devContainer.SecurityContext)
	assert.Equal(t, new(false), devContainer.SecurityContext.ReadOnlyRootFilesystem)
	require.NotNil(t, devContainer.LivenessProbe)
	assert.Equal(t, int32(5), devContainer.LivenessProbe.PeriodSeconds)
	assert.Equal(t, int32(60), devContainer.LivenessProbe.FailureThreshold)
	require.NotNil(t, devContainer.ReadinessProbe)
	assert.Equal(t, int32(5), devContainer.ReadinessProbe.PeriodSeconds)
	assert.Equal(t, int32(60), devContainer.ReadinessProbe.FailureThreshold)

	// Pins production behavior unchanged.
	prodDeploy := &appsv1.Deployment{}
	mutateDeployment(prodDeploy, cr, &def, nil, "")
	prodContainer := prodDeploy.Spec.Template.Spec.Containers[0]
	require.NotNil(t, prodContainer.SecurityContext)
	assert.Equal(t, new(true), prodContainer.SecurityContext.ReadOnlyRootFilesystem)
	require.NotNil(t, prodContainer.LivenessProbe)
	assert.Equal(t, int32(10), prodContainer.LivenessProbe.PeriodSeconds)
	assert.Equal(t, int32(3), prodContainer.LivenessProbe.FailureThreshold)
	require.NotNil(t, prodContainer.ReadinessProbe)
	assert.Equal(t, int32(10), prodContainer.ReadinessProbe.PeriodSeconds)
	assert.Equal(t, int32(3), prodContainer.ReadinessProbe.FailureThreshold)
}

func TestMutateDeployment_DevModeMeetsRestrictedPodSecurity(t *testing.T) {
	cr := newInstallation("acme--cert-manager", "acme", "cert-manager")
	def := pluginruntime.PluginDefinition{
		Spec: pluginruntime.PluginSpec{Image: "repo@sha256:" + strings.Repeat("a", 64)},
	}
	devImage := "localhost:5112/foo-plugin-dev:abc@sha256:" + strings.Repeat("a", 64)

	deploy := &appsv1.Deployment{}
	mutateDeployment(deploy, cr, &def, nil, devImage)

	evaluator, err := policy.NewEvaluator(policy.DefaultChecks(), nil)
	require.NoError(t, err)
	lv := psaapi.LevelVersion{Level: psaapi.LevelRestricted, Version: psaapi.LatestVersion()}
	for _, res := range evaluator.EvaluatePod(lv, &deploy.Spec.Template.ObjectMeta, &deploy.Spec.Template.Spec) {
		assert.True(t, res.Allowed, "%s: %s", res.ForbiddenReason, res.ForbiddenDetail)
	}
}

func TestMutateNamespace_EnforcesRestrictedPodSecurity(t *testing.T) {
	cr := newInstallation("acme--cert-manager", "acme", "cert-manager")

	ns := &corev1.Namespace{}
	mutateNamespace(ns, cr)

	assert.Equal(t, "restricted", ns.Labels["pod-security.kubernetes.io/enforce"])
	assert.Equal(t, "plugin-controller", ns.Labels["app.kubernetes.io/managed-by"])
}
