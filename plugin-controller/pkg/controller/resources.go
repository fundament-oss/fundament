package controller

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/fundament-oss/fundament/common/kubename"
	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

const (
	labelManagedBy    = "app.kubernetes.io/managed-by"
	labelInstallation = "plugins.fundament.io/installation"
	labelOrganization = "plugins.fundament.io/organization"
	labelPlugin       = "plugins.fundament.io/plugin"
	managedByValue    = "plugin-controller"

	// childResourceName names every namespace-scoped child, shared with
	// plugin-proxy and kube-api-proxy so the three cannot disagree.
	childResourceName = kubename.PluginChildName
)

// maxInstallationNameLen caps metadata.name so the cluster-scoped
// "plugin-<name>-scope" ClusterRole stays inside the 253-char limit.
const maxInstallationNameLen = 239

// installationNameSeparator joins the organization and plugin names into
// metadata.name. A double dash, because neither half may contain one — that is
// what makes the pair recoverable and keeps two different pairs from colliding
// on the same name.
const installationNameSeparator = "--"

// installationNameRegex is the DNS-1123 *label* charset without the 63-char
// limit. The namespace is derived from this name, so dots (legal in a subdomain)
// must not appear.
var installationNameRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// validateInstallation checks that the CR's identity is well-formed: the
// organization and plugin names are present and metadata.name is exactly
// "<organizationName>--<pluginName>", which is what makes the apiserver enforce
// uniqueness of the pair.
//
// The separator is a double dash, and neither half may contain one (enforced by
// organizations_ck_name and plugins_ck_name): a single dash would be ambiguous,
// since ("system", "cert-manager") and ("system-cert", "manager") would both
// produce "system-cert-manager" and collide on a name neither owns.
func validateInstallation(cr *pluginsv1.PluginInstallation) error {
	org := cr.Spec.DefinitionRef.OrganizationName
	plugin := cr.Spec.DefinitionRef.PluginName
	if org == "" {
		return fmt.Errorf("spec.definitionRef.organizationName must not be empty")
	}
	if plugin == "" {
		return fmt.Errorf("spec.definitionRef.pluginName must not be empty")
	}
	if strings.Contains(org, installationNameSeparator) {
		return fmt.Errorf("spec.definitionRef.organizationName %q must not contain %q", org, installationNameSeparator)
	}
	if strings.Contains(plugin, installationNameSeparator) {
		return fmt.Errorf("spec.definitionRef.pluginName %q must not contain %q", plugin, installationNameSeparator)
	}
	want := org + installationNameSeparator + plugin
	if cr.Name != want {
		return fmt.Errorf("metadata.name %q must equal %q (\"<organizationName>--<pluginName>\")", cr.Name, want)
	}
	if len(cr.Name) > maxInstallationNameLen {
		return fmt.Errorf("metadata.name %q exceeds maximum length of %d characters", cr.Name, maxInstallationNameLen)
	}
	if !installationNameRegex.MatchString(cr.Name) {
		return fmt.Errorf("metadata.name %q must be lowercase alphanumeric or '-', starting and ending with an alphanumeric character", cr.Name)
	}
	return nil
}

// pluginNamespace derives the plugin's namespace. plugin-proxy and kube-api-proxy
// derive it the same way through the shared helper, so the three cannot drift.
func pluginNamespace(installationName string) string {
	return kubename.PluginNamespace(installationName)
}

func childLabels(cr *pluginsv1.PluginInstallation) map[string]string {
	labels := map[string]string{
		labelManagedBy:    managedByValue,
		labelInstallation: pluginNamespace(cr.Name),
	}
	// Decorative, and label values cap at 63 chars — omit rather than emit an
	// invalid object when a name is long.
	if org := cr.Spec.DefinitionRef.OrganizationName; len(org) <= validation.DNS1123LabelMaxLength {
		labels[labelOrganization] = org
	}
	if plugin := cr.Spec.DefinitionRef.PluginName; len(plugin) <= validation.DNS1123LabelMaxLength {
		labels[labelPlugin] = plugin
	}
	return labels
}

// mergeLabels merges src labels into dst, initializing the map if needed.
// Returns the (possibly new) map.
func mergeLabels(dst, src map[string]string) map[string]string {
	if dst == nil {
		dst = make(map[string]string, len(src))
	}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// selectorLabels is the immutable Deployment selector, so it carries only the
// bounded, unique installation slug.
func selectorLabels(cr *pluginsv1.PluginInstallation) map[string]string {
	return map[string]string{
		labelInstallation: pluginNamespace(cr.Name),
	}
}

// mutateNamespace applies the desired state to an existing or empty Namespace.
func mutateNamespace(ns *corev1.Namespace, cr *pluginsv1.PluginInstallation) {
	ns.Labels = mergeLabels(ns.Labels, childLabels(cr))
}

// mutateServiceAccount applies the desired state to an existing or empty ServiceAccount.
func mutateServiceAccount(sa *corev1.ServiceAccount, cr *pluginsv1.PluginInstallation) {
	sa.Labels = mergeLabels(sa.Labels, childLabels(cr))
}

// mutateRoleBinding binds the plugin's ServiceAccount to the built-in admin ClusterRole
// within the plugin's namespace.
func mutateRoleBinding(rb *rbacv1.RoleBinding, cr *pluginsv1.PluginInstallation) {
	rb.Labels = childLabels(cr)
	rb.RoleRef = rbacv1.RoleRef{
		APIGroup: rbacv1.GroupName,
		Kind:     "ClusterRole",
		Name:     "admin",
	}
	rb.Subjects = []rbacv1.Subject{
		{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      childResourceName,
			Namespace: pluginNamespace(cr.Name),
		},
	}
}

// pluginScopeClusterRoleName is the name of the ClusterRole materialised from
// the pinned PluginDefinition. It is bound to the per-installation plugin SA;
// the cluster's RBAC on that SA is the plugin-scope half of FUN-17's user ∩
// plugin enforcement.
func pluginScopeClusterRoleName(installationName string) string {
	return fmt.Sprintf("plugin-%s-scope", installationName)
}

// mutatePluginScopeClusterRole materialises the plugin's declared
// permissions.rbac (parsed from the fetched PluginDefinition manifest) into a
// real ClusterRole. The cluster's own RBAC engine evaluates this when
// kube-api-proxy injects the plugin SA token — there is no bespoke matcher
// anywhere.
func mutatePluginScopeClusterRole(role *rbacv1.ClusterRole, cr *pluginsv1.PluginInstallation, rules []pluginruntime.PolicyRule) {
	role.Labels = mergeLabels(role.Labels, childLabels(cr))
	role.Rules = make([]rbacv1.PolicyRule, 0, len(rules))
	for _, rule := range rules {
		role.Rules = append(role.Rules, rbacv1.PolicyRule{
			APIGroups:     rule.APIGroups,
			Resources:     rule.Resources,
			Verbs:         rule.Verbs,
			ResourceNames: rule.ResourceNames,
		})
	}
}

// mutatePluginScopeClusterRoleBinding binds the materialised ClusterRole to the
// per-installation plugin ServiceAccount.
func mutatePluginScopeClusterRoleBinding(crb *rbacv1.ClusterRoleBinding, cr *pluginsv1.PluginInstallation) {
	crb.Labels = mergeLabels(crb.Labels, childLabels(cr))
	crb.RoleRef = rbacv1.RoleRef{
		APIGroup: rbacv1.GroupName,
		Kind:     "ClusterRole",
		Name:     pluginScopeClusterRoleName(cr.Name),
	}
	crb.Subjects = []rbacv1.Subject{{
		Kind:      rbacv1.ServiceAccountKind,
		Name:      childResourceName,
		Namespace: pluginNamespace(cr.Name),
	}}
}

// mutateDeployment applies the desired state to an existing or empty
// Deployment. Image and pull policy are sourced from the parsed
// PluginDefinition — never from the CR — so the hash-verified manifest is the
// sole gate on what image runs.
func mutateDeployment(deploy *appsv1.Deployment, cr *pluginsv1.PluginInstallation, def *pluginruntime.PluginDefinition, fundEnvVars []corev1.EnvVar) {
	labels := childLabels(cr)
	replicas := int32(1)

	envVars := make([]corev1.EnvVar, 0, len(cr.Spec.Config)+len(fundEnvVars))
	envVars = append(envVars, fundEnvVars...)
	configKeys := make([]string, 0, len(cr.Spec.Config))
	for k := range cr.Spec.Config {
		configKeys = append(configKeys, k)
	}
	sort.Strings(configKeys)
	for _, k := range configKeys {
		envVars = append(envVars, corev1.EnvVar{Name: "FUNP_" + k, Value: cr.Spec.Config[k]})
	}

	deploy.Labels = labels
	deploy.Spec.Replicas = &replicas
	deploy.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: selectorLabels(cr),
	}
	deploy.Spec.Template = corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: labels,
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: childResourceName,
			Containers: []corev1.Container{
				{
					Name:            childResourceName,
					Image:           def.Spec.Image,
					ImagePullPolicy: corev1.PullPolicy(def.Spec.ImagePullPolicy),
					Ports: []corev1.ContainerPort{
						{
							Name:          "http",
							ContainerPort: 8080,
							Protocol:      corev1.ProtocolTCP,
						},
					},
					Env: envVars,
					LivenessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/livez",
								Port: intstr.FromString("http"),
							},
						},
						InitialDelaySeconds: 5,
						PeriodSeconds:       10,
					},
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/readyz",
								Port: intstr.FromString("http"),
							},
						},
						InitialDelaySeconds: 10,
						PeriodSeconds:       10,
					},
				},
			},
		},
	}
}

// mutateService applies the desired state to an existing or empty Service.
func mutateService(svc *corev1.Service, cr *pluginsv1.PluginInstallation) {
	svc.Labels = childLabels(cr)
	svc.Spec.Selector = selectorLabels(cr)
	// Route to the pod as soon as its metadata server is up, before /readyz
	// passes. The controller reaches GetDefinition through this Service to
	// materialise the plugin's RBAC scope; a plugin can't become Ready until it
	// has installed, and it can't install without that scope — so gating the
	// Service on readiness would deadlock the bootstrap.
	//
	// TODO(FUN-*): this Service also carries live user data-plane traffic (asset
	// fetches), so publishing not-ready addresses means user requests can hit
	// not-ready pods during rollouts/crash-loops (→ transient 502s). Drop this
	// flag once GetDefinition moves to the DB and the controller no longer needs
	// to dial the not-ready pod, letting the Service gate on readiness normally.
	svc.Spec.PublishNotReadyAddresses = true
	svc.Spec.Ports = []corev1.ServicePort{
		{
			Name:       "http",
			Port:       8080,
			TargetPort: intstr.FromString("http"),
		},
	}
}
