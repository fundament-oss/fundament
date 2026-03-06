package controller

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
)

const (
	labelManagedBy             = "app.kubernetes.io/managed-by"
	labelPlugin                = "plugins.fundament.io/plugin"
	labelInstallationName      = "plugins.fundament.io/installation-name"
	labelInstallationNamespace = "plugins.fundament.io/installation-namespace"
	managedByValue             = "plugin-controller"
)

func childName(pluginName string) string {
	return fmt.Sprintf("plugin-%s", pluginName)
}

func pluginNamespace(pluginName string) string {
	return fmt.Sprintf("plugin-%s", pluginName)
}

func childLabels(cr *pluginsv1.PluginInstallation) map[string]string {
	return map[string]string{
		labelManagedBy:             managedByValue,
		labelPlugin:                cr.Spec.PluginName,
		labelInstallationName:      cr.Name,
		labelInstallationNamespace: cr.Namespace,
	}
}

func selectorLabels(cr *pluginsv1.PluginInstallation) map[string]string {
	return map[string]string{
		labelPlugin:           cr.Spec.PluginName,
		labelInstallationName: cr.Name,
	}
}

// mutateNamespace applies the desired state to an existing or empty Namespace.
func mutateNamespace(ns *corev1.Namespace, cr *pluginsv1.PluginInstallation) {
	ns.Labels = childLabels(cr)
}

// mutateServiceAccount applies the desired state to an existing or empty ServiceAccount.
func mutateServiceAccount(sa *corev1.ServiceAccount, cr *pluginsv1.PluginInstallation) {
	sa.Labels = childLabels(cr)
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
			Name:      childName(cr.Spec.PluginName),
			Namespace: pluginNamespace(cr.Spec.PluginName),
		},
	}
}

// mutateClusterRoleBinding binds the plugin's ServiceAccount to a ClusterRole
// at cluster scope, for plugins that need cross-namespace or cluster-wide access.
func mutateClusterRoleBinding(crb *rbacv1.ClusterRoleBinding, cr *pluginsv1.PluginInstallation, clusterRoleName string) {
	crb.Labels = childLabels(cr)
	crb.RoleRef = rbacv1.RoleRef{
		APIGroup: rbacv1.GroupName,
		Kind:     "ClusterRole",
		Name:     clusterRoleName,
	}
	crb.Subjects = []rbacv1.Subject{
		{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      childName(cr.Spec.PluginName),
			Namespace: pluginNamespace(cr.Spec.PluginName),
		},
	}
}

// clusterRoleBindingName returns a unique name for a plugin's ClusterRoleBinding.
func clusterRoleBindingName(pluginName, clusterRoleName string) string {
	return fmt.Sprintf("plugin-%s-%s", pluginName, clusterRoleName)
}

// mutateDeployment applies the desired state to an existing or empty Deployment.
func mutateDeployment(deploy *appsv1.Deployment, cr *pluginsv1.PluginInstallation, fundEnvVars []corev1.EnvVar) {
	labels := childLabels(cr)
	replicas := int32(1)

	envVars := make([]corev1.EnvVar, 0, len(cr.Spec.Config)+len(fundEnvVars))
	envVars = append(envVars, fundEnvVars...)
	for k, v := range cr.Spec.Config {
		envVars = append(envVars, corev1.EnvVar{Name: k, Value: v})
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
			ServiceAccountName: childName(cr.Spec.PluginName),
			Containers: []corev1.Container{
				{
					Name:  cr.Spec.PluginName,
					Image: cr.Spec.Image,
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
								Path: "/healthz",
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
	svc.Spec.Ports = []corev1.ServicePort{
		{
			Name:       "http",
			Port:       8080,
			TargetPort: intstr.FromString("http"),
		},
	}
}
