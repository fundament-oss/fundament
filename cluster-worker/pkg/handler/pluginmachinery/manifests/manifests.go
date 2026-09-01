// Package manifests holds everything the pluginmachinery handler applies onto
// a shoot: an embedded copy of the PluginInstallation CRD and typed builders
// for the plugin-controller ServiceAccount, ClusterRole, ClusterRoleBinding
// and Deployment.
//
// The CRD's source of truth is the chart file; the copy here is refreshed by
// go generate and pinned byte-for-byte by TestCRDMatchesChart. The builders
// mirror charts/fundament/templates/plugin-controller.yaml — the chart file is
// Helm-templated, so it cannot be embedded directly; TestClusterRoleRulesMatchChart
// pins the RBAC rules against the chart's plain-YAML rules block instead.
package manifests

//go:generate cp ../../../../../charts/fundament/crds/plugininstallations.plugins.fundament.io.yaml plugininstallations.plugins.fundament.io.yaml

import (
	_ "embed"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// CRD is the PluginInstallation CustomResourceDefinition manifest, generated
// from charts/fundament/crds/ (see the go:generate directive above).
//
//go:embed plugininstallations.plugins.fundament.io.yaml
var CRD []byte

const (
	// Namespace is where the plugin-controller runs on a shoot.
	Namespace = "fundament-system"

	// DeploymentName is the shoot-side plugin-controller Deployment and
	// ServiceAccount name.
	DeploymentName = "plugin-controller"

	// ClusterRoleName names both the ClusterRole and the ClusterRoleBinding,
	// following the fundament:<component> convention used for shoot-side RBAC.
	ClusterRoleName = "fundament:plugin-controller"

	// LabelManagedBy marks every resource this package builds; the value
	// matches the namespace handler's convention.
	LabelManagedBy = "fundament.io/managed-by"
	// ManagedByValue is the value of LabelManagedBy.
	ManagedByValue = "cluster-worker"

	// componentLabelValue mirrors the chart's app.kubernetes.io/component and
	// doubles as the Deployment's (immutable) selector value.
	componentLabelValue = "plugin-controller"

	healthPort = 8097
)

// Labels returns the label set applied to every shoot-side plugin machinery
// resource.
func Labels() map[string]string {
	return map[string]string{
		LabelManagedBy:                ManagedByValue,
		"app.kubernetes.io/component": componentLabelValue,
	}
}

// ClusterRoleRules are the permissions the shoot-side plugin-controller needs.
//
// KEEP IN SYNC with the rules block of
// charts/fundament/templates/plugin-controller.yaml (the management-cluster
// deployment of the same controller); TestClusterRoleRulesMatchChart enforces
// the equivalence.
func ClusterRoleRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{"plugins.fundament.io"},
			Resources: []string{"plugininstallations"},
			Verbs:     []string{"get", "list", "watch", "update", "patch"},
		},
		{
			APIGroups: []string{"plugins.fundament.io"},
			Resources: []string{"plugininstallations/status"},
			Verbs:     []string{"get", "update", "patch"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"namespaces", "serviceaccounts", "services"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
		},
		{
			APIGroups: []string{"apps"},
			Resources: []string{"deployments"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
		},
		{
			APIGroups: []string{"rbac.authorization.k8s.io"},
			Resources: []string{"rolebindings"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete", "bind"},
		},
		{
			APIGroups: []string{"rbac.authorization.k8s.io"},
			Resources: []string{"clusterroles"},
			// escalate: plugin-scope ClusterRoles carry rules materialised from
			// the pinned PluginDefinition (FUN-17), which the controller SA does
			// not itself hold. Escalation is bounded by the definition-hash gate;
			// see the chart template for the full rationale.
			Verbs: []string{"get", "list", "watch", "create", "update", "patch", "delete", "bind", "escalate"},
		},
		{
			APIGroups: []string{"rbac.authorization.k8s.io"},
			Resources: []string{"clusterrolebindings"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
		},
	}
}

// DeploymentParams are the per-shoot inputs of the plugin-controller Deployment.
type DeploymentParams struct {
	// Image is the plugin-controller container image, resolvable from shoot nodes.
	Image string
	// ClusterID is the fundament cluster UUID, stamped as FUNDAMENT_CLUSTER_ID
	// and (until a real installation-id concept exists) FUNDAMENT_INSTALL_ID.
	ClusterID string
	// OrganizationID is the owning organization UUID.
	OrganizationID string
	// OrganizationAPIURL is the externally routable organization-api base URL
	// (FUN-19: the controller runs outside the management cluster).
	OrganizationAPIURL string
	// AllowUnpinnedHash skips definition-hash verification. Local dev only;
	// never set for production shoots.
	AllowUnpinnedHash bool
	// LogLevel is the controller's LOG_LEVEL value; empty means the
	// controller's default.
	LogLevel string
}

// Deployment builds the shoot-side plugin-controller Deployment. It mirrors
// the chart's management-cluster Deployment with fixed names and real
// per-shoot identity instead of Release.Name placeholders.
func Deployment(params *DeploymentParams) *appsv1.Deployment {
	env := []corev1.EnvVar{
		{
			Name: "NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"},
			},
		},
		{Name: "FUNDAMENT_CLUSTER_ID", Value: params.ClusterID},
		// No platform concept of an installation id exists yet; the value is
		// required (plugin-sdk hard-fails without it) but consumed by nothing,
		// so the cluster UUID stands in until a real concept lands.
		{Name: "FUNDAMENT_INSTALL_ID", Value: params.ClusterID},
		{Name: "FUNDAMENT_ORGANIZATION_ID", Value: params.OrganizationID},
		{Name: "ORGANIZATION_API_URL", Value: params.OrganizationAPIURL},
	}
	if params.LogLevel != "" {
		env = append(env, corev1.EnvVar{Name: "LOG_LEVEL", Value: params.LogLevel})
	}
	if params.AllowUnpinnedHash {
		env = append(env, corev1.EnvVar{Name: "PLUGIN_CONTROLLER_ALLOW_UNPINNED_HASH", Value: "true"})
	}

	selectorLabels := map[string]string{"app.kubernetes.io/component": componentLabelValue}
	replicas := int32(1)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DeploymentName,
			Namespace: Namespace,
			Labels:    Labels(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: selectorLabels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: selectorLabels},
				Spec: corev1.PodSpec{
					ServiceAccountName: DeploymentName,
					Containers: []corev1.Container{
						{
							Name:  "plugin-controller",
							Image: params.Image,
							Env:   env,
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/livez",
										Port: intstr.FromInt32(healthPort),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/readyz",
										Port: intstr.FromInt32(healthPort),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
							},
						},
					},
				},
			},
		},
	}
}

// Validate reports the first missing required Deployment parameter.
func (p *DeploymentParams) Validate() error {
	if p.Image == "" {
		return fmt.Errorf("plugin-controller image is empty")
	}
	if p.ClusterID == "" {
		return fmt.Errorf("cluster id is empty")
	}
	if p.OrganizationID == "" {
		return fmt.Errorf("organization id is empty")
	}
	if p.OrganizationAPIURL == "" {
		return fmt.Errorf("organization-api URL is empty")
	}
	return nil
}
