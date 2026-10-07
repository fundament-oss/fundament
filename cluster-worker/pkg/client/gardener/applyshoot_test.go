package gardener

import (
	"context"
	"log/slog"
	"testing"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// applyShootCalls records how often each intercepted call ran, so a test can
// assert both that the update was retried and that the label lookup was not.
type applyShootCalls struct {
	lists   int
	gets    int
	updates int
}

// shootConflict is the error gardener's API server returns when its controllers
// stamp the shoot (status, finalizers) between our read and our update.
func shootConflict(name string) error {
	return apierrors.NewConflict(
		schema.GroupResource{Group: gardencorev1beta1.GroupName, Resource: "shoots"},
		name,
		assert.AnError,
	)
}

// applyShootTestClient serves cluster's shoot from a fake client. updateErr is
// consulted on every Update with the 1-based attempt number; a non-nil result is
// returned instead of writing, so a test can fail the first attempt only, every
// attempt, or none.
func applyShootTestClient(t *testing.T, cluster *ClusterToSync, updateErr func(attempt int) error) (*RealClient, *applyShootCalls) {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, gardencorev1beta1.AddToScheme(scheme))

	existing := &gardencorev1beta1.Shoot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.ShootName,
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				LabelClusterID:      cluster.ID.String(),
				LabelOrganizationID: cluster.OrganizationID.String(),
			},
		},
		Spec: gardencorev1beta1.ShootSpec{
			Region:     "local",
			Kubernetes: gardencorev1beta1.Kubernetes{Version: "1.35.5"},
		},
	}

	calls := &applyShootCalls{}
	tracked := interceptor.Funcs{
		List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			calls.lists++
			return c.List(ctx, list, opts...)
		},
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			calls.gets++
			return c.Get(ctx, key, obj, opts...)
		},
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			calls.updates++
			if err := updateErr(calls.updates); err != nil {
				return err
			}
			return c.Update(ctx, obj, opts...)
		},
	}

	r := &RealClient{
		client: interceptor.NewClient(
			fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build(),
			tracked,
		),
		provider: NewProviderConfig(),
		logger:   slog.Default(),
	}
	return r, calls
}

func applyShootTestCluster() *ClusterToSync {
	cluster := testCluster()
	cluster.Namespace = "garden-local"
	return cluster
}

// A conflict on the first update is retried transparently: the caller sees no
// error and the shoot ends up carrying the desired spec.
func TestApplyShoot_RetriesOnConflict(t *testing.T) {
	t.Parallel()

	cluster := applyShootTestCluster()
	cluster.KubernetesVersion = "1.35.6"

	r, calls := applyShootTestClient(t, cluster, func(attempt int) error {
		if attempt == 1 {
			return shootConflict(cluster.ShootName)
		}
		return nil
	})

	require.NoError(t, r.ApplyShoot(t.Context(), cluster))

	// Two updates: the conflicting one and the retry. The label lookup runs once -
	// the retry re-reads by key, so it gets a Get per attempt and no second List.
	assert.Equal(t, 2, calls.updates)
	assert.Equal(t, 2, calls.gets)
	assert.Equal(t, 1, calls.lists)

	got := &gardencorev1beta1.Shoot{}
	require.NoError(t, r.client.Get(t.Context(),
		client.ObjectKey{Namespace: cluster.Namespace, Name: cluster.ShootName}, got))
	assert.Equal(t, "1.35.6", got.Spec.Kubernetes.Version)
	assert.Equal(t, cluster.Name, got.Annotations[AnnotationClusterName])
}

// Conflicts that outlast the retry budget surface as an error rather than
// silently dropping the update.
func TestApplyShoot_GivesUpAfterPersistentConflicts(t *testing.T) {
	t.Parallel()

	cluster := applyShootTestCluster()

	r, calls := applyShootTestClient(t, cluster, func(int) error {
		return shootConflict(cluster.ShootName)
	})

	err := r.ApplyShoot(t.Context(), cluster)
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to update shoot")
	assert.True(t, apierrors.IsConflict(err), "the conflict should stay recognizable, got %v", err)
	assert.Equal(t, 1, calls.lists)
	assert.Greater(t, calls.updates, 1, "the update should have been retried")
}

// A non-conflict update error is returned on the first attempt: RetryOnConflict
// retries conflicts only.
func TestApplyShoot_DoesNotRetryOtherErrors(t *testing.T) {
	t.Parallel()

	cluster := applyShootTestCluster()

	r, calls := applyShootTestClient(t, cluster, func(int) error {
		return apierrors.NewInternalError(assert.AnError)
	})

	err := r.ApplyShoot(t.Context(), cluster)
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to update shoot")
	assert.Equal(t, 1, calls.updates)
}
