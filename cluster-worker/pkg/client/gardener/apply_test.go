package gardener

import (
	"context"
	"io"
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

// newApplyTestClient returns a RealClient whose fake garden holds the given
// Shoot and fails the first failUpdates Update calls with a conflict.
func newApplyTestClient(t *testing.T, shoot *gardencorev1beta1.Shoot, failUpdates int) (*RealClient, *int) {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, gardencorev1beta1.AddToScheme(scheme))

	updates := 0
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(shoot).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				updates++
				if updates <= failUpdates {
					return apierrors.NewConflict(schema.GroupResource{Group: "core.gardener.cloud", Resource: "shoots"}, obj.GetName(),
						assert.AnError)
				}
				return c.Update(ctx, obj, opts...)
			},
		}).
		Build()

	return &RealClient{
		client:   c,
		provider: NewProviderConfig(),
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, &updates
}

func applyTestShoot(cluster *ClusterToSync) *gardencorev1beta1.Shoot {
	return &gardencorev1beta1.Shoot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.ShootName,
			Namespace: cluster.Namespace,
			Labels:    map[string]string{LabelClusterID: cluster.ID.String()},
		},
		Spec: gardencorev1beta1.ShootSpec{Kubernetes: gardencorev1beta1.Kubernetes{Version: "1.34.0"}},
	}
}

func TestApplyShootRetriesOnConflict(t *testing.T) {
	t.Parallel()

	cluster := testCluster()
	r, updates := newApplyTestClient(t, applyTestShoot(cluster), 1)

	err := r.ApplyShoot(t.Context(), cluster)
	require.NoError(t, err)

	assert.Equal(t, 2, *updates, "one conflict, then a successful update")
	stored := &gardencorev1beta1.Shoot{}
	require.NoError(t, r.client.Get(t.Context(), client.ObjectKey{Namespace: cluster.Namespace, Name: cluster.ShootName}, stored))
	assert.Equal(t, cluster.KubernetesVersion, stored.Spec.Kubernetes.Version)
}

func TestApplyShootGivesUpOnPersistentConflict(t *testing.T) {
	t.Parallel()

	cluster := testCluster()
	r, _ := newApplyTestClient(t, applyTestShoot(cluster), 100)

	err := r.ApplyShoot(t.Context(), cluster)
	require.Error(t, err)
	assert.True(t, apierrors.IsConflict(err), "the conflict surfaces once retries are exhausted")
}
