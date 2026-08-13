package shoot

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

// realAccessWith returns a RealShootAccess whose clientForCluster yields the
// given fake clientset, so the Ensure* paths can be exercised without Gardener.
func realAccessWith(t *testing.T, cs kubernetes.Interface) *RealShootAccess {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	return &RealShootAccess{
		logger: logger,
		newClient: func(context.Context, uuid.UUID) (kubernetes.Interface, error) {
			return cs, nil
		},
	}
}

// A pre-existing ServiceAccount with nil Labels/Annotations must not panic when
// EnsureServiceAccount merges the desired set onto it: maps.Copy into a nil map
// panics. Regression test for the nil-map guard on the AlreadyExists path.
func TestEnsureServiceAccount_MergesOntoNilMaps(t *testing.T) {
	t.Parallel()
	existing := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "sa", Namespace: "ns"}, // Labels/Annotations are nil
	}
	cs := fake.NewClientset(existing)
	r := realAccessWith(t, cs)

	err := r.EnsureServiceAccount(context.Background(), uuid.New(), "ns", "sa",
		map[string]string{"fundament.io/user-id": "u1"},
		map[string]string{"note": "managed"})
	require.NoError(t, err)

	got, err := cs.CoreV1().ServiceAccounts("ns").Get(context.Background(), "sa", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, "u1", got.Labels["fundament.io/user-id"])
	require.Equal(t, "managed", got.Annotations["note"])
}

// Same nil-map guard for the ClusterRoleBinding update path. The pre-existing CRB
// shares the desired RoleRef (so it is not recreated) but has nil Labels: the
// merge must fill them in without panicking.
func TestEnsureClusterRoleBinding_MergesOntoNilMaps(t *testing.T) {
	t.Parallel()
	existing := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "crb"}, // Labels/Annotations are nil
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      "sa",
			Namespace: "ns",
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
	}
	cs := fake.NewClientset(existing)
	r := realAccessWith(t, cs)

	err := r.EnsureClusterRoleBinding(context.Background(), uuid.New(), "crb", "cluster-admin", "ns", "sa",
		map[string]string{"fundament.io/user-id": "u1"},
		map[string]string{"note": "managed"})
	require.NoError(t, err)

	got, err := cs.RbacV1().ClusterRoleBindings().Get(context.Background(), "crb", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, "u1", got.Labels["fundament.io/user-id"])
	require.Equal(t, "managed", got.Annotations["note"])
}

// realAccessWithAPIExt returns a RealShootAccess whose apiextensions client is
// the given fake, for exercising EnsureCRD without Gardener.
func realAccessWithAPIExt(t *testing.T, cs apiextensionsclientset.Interface) *RealShootAccess {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	return &RealShootAccess{
		logger: logger,
		newAPIExtClient: func(context.Context, uuid.UUID) (apiextensionsclientset.Interface, error) {
			return cs, nil
		},
	}
}

const testCRDManifest = `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.example.io
spec:
  group: example.io
  names:
    kind: Widget
    listKind: WidgetList
    plural: widgets
    singular: widget
  scope: Cluster
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
`

func TestEnsureCRD_CreatesFromManifest(t *testing.T) {
	t.Parallel()
	cs := apiextensionsfake.NewSimpleClientset()
	r := realAccessWithAPIExt(t, cs)

	err := r.EnsureCRD(context.Background(), uuid.New(), []byte(testCRDManifest))
	require.NoError(t, err)

	got, err := cs.ApiextensionsV1().CustomResourceDefinitions().Get(context.Background(), "widgets.example.io", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "example.io", got.Spec.Group)
	assert.Equal(t, "Widget", got.Spec.Names.Kind)
}

// An existing CRD with a stale spec is updated in place — EnsureCRD is the
// upgrade channel for shoots (Helm only applies crds/ at install time).
func TestEnsureCRD_UpdatesExistingSpec(t *testing.T) {
	t.Parallel()
	stale := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "widgets.example.io"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "example.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind: "OldWidget", ListKind: "OldWidgetList", Plural: "widgets", Singular: "widget",
			},
			Scope: apiextensionsv1.ClusterScoped,
		},
	}
	cs := apiextensionsfake.NewSimpleClientset(stale)
	r := realAccessWithAPIExt(t, cs)

	err := r.EnsureCRD(context.Background(), uuid.New(), []byte(testCRDManifest))
	require.NoError(t, err)

	got, err := cs.ApiextensionsV1().CustomResourceDefinitions().Get(context.Background(), "widgets.example.io", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "Widget", got.Spec.Names.Kind)
	require.Len(t, got.Spec.Versions, 1)
	assert.Equal(t, "v1", got.Spec.Versions[0].Name)
}

func TestEnsureCRD_InvalidManifest(t *testing.T) {
	t.Parallel()
	r := realAccessWithAPIExt(t, apiextensionsfake.NewSimpleClientset())

	err := r.EnsureCRD(context.Background(), uuid.New(), []byte("\tnot yaml"))
	require.Error(t, err)
}

func TestEnsureClusterRole_CreatesAndUpdatesRules(t *testing.T) {
	t.Parallel()
	cs := fake.NewClientset()
	r := realAccessWith(t, cs)

	rules := make([]rbacv1.PolicyRule, 0, 2)
	rules = append(rules, rbacv1.PolicyRule{
		APIGroups: []string{"plugins.fundament.io"},
		Resources: []string{"plugininstallations"},
		Verbs:     []string{"get", "list", "watch"},
	})
	labels := map[string]string{"fundament.io/managed-by": "cluster-worker"}

	err := r.EnsureClusterRole(context.Background(), uuid.New(), "fundament:plugin-controller", rules, labels)
	require.NoError(t, err)

	got, err := cs.RbacV1().ClusterRoles().Get(context.Background(), "fundament:plugin-controller", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, rules, got.Rules)
	assert.Equal(t, "cluster-worker", got.Labels["fundament.io/managed-by"])

	// Second ensure with extended rules replaces the rule set in place.
	rules = append(rules, rbacv1.PolicyRule{
		APIGroups: []string{"apps"},
		Resources: []string{"deployments"},
		Verbs:     []string{"create"},
	})
	err = r.EnsureClusterRole(context.Background(), uuid.New(), "fundament:plugin-controller", rules, labels)
	require.NoError(t, err)

	got, err = cs.RbacV1().ClusterRoles().Get(context.Background(), "fundament:plugin-controller", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, rules, got.Rules)
}

func testDeployment(selectorValue string) *appsv1.Deployment {
	labels := map[string]string{"app": selectorValue}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "plugin-controller", Namespace: "fundament-system"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main", Image: "example/app:v1"}},
				},
			},
		},
	}
}

func TestEnsureDeployment_CreatesAndUpdates(t *testing.T) {
	t.Parallel()
	cs := fake.NewClientset()
	r := realAccessWith(t, cs)

	err := r.EnsureDeployment(context.Background(), uuid.New(), testDeployment("pc"))
	require.NoError(t, err)

	// Update: same selector, new image — updated in place.
	desired := testDeployment("pc")
	desired.Spec.Template.Spec.Containers[0].Image = "example/app:v2"
	err = r.EnsureDeployment(context.Background(), uuid.New(), desired)
	require.NoError(t, err)

	got, err := cs.AppsV1().Deployments("fundament-system").Get(context.Background(), "plugin-controller", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "example/app:v2", got.Spec.Template.Spec.Containers[0].Image)
}

// A changed label selector (immutable field) must trigger delete+recreate
// instead of an Update that the API server would reject.
func TestEnsureDeployment_RecreatesOnSelectorChange(t *testing.T) {
	t.Parallel()
	cs := fake.NewClientset(testDeployment("old"))
	r := realAccessWith(t, cs)

	err := r.EnsureDeployment(context.Background(), uuid.New(), testDeployment("new"))
	require.NoError(t, err)

	got, err := cs.AppsV1().Deployments("fundament-system").Get(context.Background(), "plugin-controller", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "new", got.Spec.Selector.MatchLabels["app"])
}

// The RoleRef is immutable: binding to a different ClusterRole must recreate
// the CRB rather than update it.
func TestEnsureClusterRoleBinding_RecreatesOnRoleChange(t *testing.T) {
	t.Parallel()
	existing := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "crb"},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "sa", Namespace: "ns"}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
	}
	cs := fake.NewClientset(existing)
	r := realAccessWith(t, cs)

	err := r.EnsureClusterRoleBinding(context.Background(), uuid.New(), "crb", "fundament:plugin-controller", "ns", "sa", nil, nil)
	require.NoError(t, err)

	got, err := cs.RbacV1().ClusterRoleBindings().Get(context.Background(), "crb", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "fundament:plugin-controller", got.RoleRef.Name)
}
