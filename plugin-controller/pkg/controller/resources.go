package controller

import (
	"fmt"
	"regexp"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

const (
	labelManagedBy        = "app.kubernetes.io/managed-by"
	labelPlugin           = "plugins.fundament.io/plugin"
	labelInstallationName = "plugins.fundament.io/installation-name"
	managedByValue        = "plugin-controller"
)

// dnsLabelRegex matches valid DNS label names (RFC 1123).
var dnsLabelRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// maxInstallationNameLen caps metadata.name so that child resource names —
// which are prefixed with "plugin-" — stay within Kubernetes' 63-character
// DNS-label limit.
const maxInstallationNameLen = 56

// validateInstallationName checks that the PluginInstallation's metadata.name
// is a valid DNS label and short enough to derive prefixed child resource
// names from it.
func validateInstallationName(name string) error {
	if name == "" {
		return fmt.Errorf("metadata.name must not be empty")
	}
	if len(name) > maxInstallationNameLen {
		return fmt.Errorf("metadata.name %q exceeds maximum length of %d characters (child resources are prefixed with %q)", name, maxInstallationNameLen, "plugin-")
	}
	if !dnsLabelRegex.MatchString(name) {
		return fmt.Errorf("metadata.name %q is not a valid DNS label (must be lowercase alphanumeric or '-', and must start and end with an alphanumeric character)", name)
	}
	return nil
}

func childName(installationName string) string {
	return fmt.Sprintf("plugin-%s", installationName)
}

func pluginNamespace(installationName string) string {
	return fmt.Sprintf("plugin-%s", installationName)
}

func childLabels(cr *pluginsv1.PluginInstallation) map[string]string {
	return map[string]string{
		labelManagedBy:        managedByValue,
		labelPlugin:           cr.Name,
		labelInstallationName: cr.Name,
	}
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

func selectorLabels(cr *pluginsv1.PluginInstallation) map[string]string {
	return map[string]string{
		labelPlugin:           cr.Name,
		labelInstallationName: cr.Name,
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
			Name:      childName(cr.Name),
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
		Name:      childName(cr.Name),
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
			ServiceAccountName: childName(cr.Name),
			Containers: []corev1.Container{
				{
					Name:            cr.Name,
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
