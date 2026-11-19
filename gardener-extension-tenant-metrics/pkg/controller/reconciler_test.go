package controller

import (
	"testing"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const testNamespace = "shoot--p--s"

func newReconciler(t *testing.T, sourceSpec map[string]any) *Reconciler {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, extensionsv1alpha1.AddToScheme(scheme))

	ext := &extensionsv1alpha1.Extension{
		ObjectMeta: metav1.ObjectMeta{Name: "fundament-tenant-metrics", Namespace: testNamespace, Generation: 1},
		Spec:       extensionsv1alpha1.ExtensionSpec{DefaultSpec: extensionsv1alpha1.DefaultSpec{Type: ExtensionType}},
	}
	source := &unstructured.Unstructured{}
	source.SetGroupVersionKind(scrapeConfigGVK)
	source.SetNamespace(testNamespace)
	source.SetName(sourceName)
	source.Object["spec"] = sourceSpec

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ext, source).WithStatusSubresource(ext).Build()
	return &Reconciler{Client: c}
}

func reconcileExtension(t *testing.T, r *Reconciler) (ctrl.Result, *extensionsv1alpha1.Extension) {
	t.Helper()
	key := types.NamespacedName{Namespace: testNamespace, Name: "fundament-tenant-metrics"}
	result, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: key})
	require.NoError(t, err)
	ext := &extensionsv1alpha1.Extension{}
	require.NoError(t, r.Get(t.Context(), key, ext))
	return result, ext
}

func targetExists(t *testing.T, c client.Client) bool {
	t.Helper()
	target := &unstructured.Unstructured{}
	target.SetGroupVersionKind(scrapeConfigGVK)
	err := c.Get(t.Context(), types.NamespacedName{Namespace: testNamespace, Name: targetName}, target)
	if apierrors.IsNotFound(err) {
		return false
	}
	require.NoError(t, err)
	return true
}

func TestReconcile_WritesTenantJob(t *testing.T) {
	r := newReconciler(t, gardenerCadvisorSpec())

	result, ext := reconcileExtension(t, r)

	assert.Equal(t, resync, result.RequeueAfter)
	assert.Equal(t, gardencorev1beta1.LastOperationStateSucceeded, ext.Status.LastOperation.State)
	assert.True(t, targetExists(t, r.Client))
}

// A Gardener upgrade that changes its cAdvisor job must not fail the Extension:
// gardenlet would stop reconciling every shoot on the seed.
func TestReconcile_UnknownShapeDoesNotFailTheExtension(t *testing.T) {
	spec := gardenerCadvisorSpec()
	spec["metricRelabelings"] = []any{}
	r := newReconciler(t, spec)

	_, ext := reconcileExtension(t, r)

	assert.Equal(t, gardencorev1beta1.LastOperationStateSucceeded, ext.Status.LastOperation.State)
	assert.Contains(t, ext.Status.LastOperation.Description, "not written")
	assert.Nil(t, ext.Status.LastError)
	assert.False(t, targetExists(t, r.Client))
}
