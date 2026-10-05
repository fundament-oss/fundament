package gardener

import (
	"log/slog"
	"testing"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var cleanupAnnotations = []string{
	v1beta1constants.AnnotationShootCleanupKubernetesResourcesFinalizeGracePeriodSeconds,
	v1beta1constants.AnnotationShootCleanupExtendedAPIsFinalizeGracePeriodSeconds,
	v1beta1constants.AnnotationShootCleanupWebhooksFinalizeGracePeriodSeconds,
}

// deletionTestShoot carries a finalizer, so the fake client's Delete leaves the
// object behind for the annotations to be read off.
func deletionTestShoot() *gardencorev1beta1.Shoot {
	return &gardencorev1beta1.Shoot{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "demo",
			Namespace:  "garden-local",
			Finalizers: []string{"gardener"},
		},
	}
}

func deletionTestClient(t *testing.T, immediate bool, shoot *gardencorev1beta1.Shoot) *RealClient {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, gardencorev1beta1.AddToScheme(scheme))

	provider := NewProviderConfig()
	provider.ImmediateClusterDeletion = immediate

	return &RealClient{
		client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(shoot).Build(),
		provider: provider,
		logger:   slog.Default(),
	}
}

// Immediate cluster deletion shortens every cleanup grace period to zero; the deletion
// confirmation is written either way.
func TestDeleteShoot_CleanupGracePeriods(t *testing.T) {
	t.Parallel()

	t.Run("immediate", func(t *testing.T) {
		t.Parallel()

		shoot := deletionTestShoot()
		r := deletionTestClient(t, true, shoot)

		require.NoError(t, r.deleteShoot(t.Context(), shoot))

		got := &gardencorev1beta1.Shoot{}
		require.NoError(t, r.client.Get(t.Context(), client.ObjectKeyFromObject(shoot), got))
		require.NotNil(t, got.DeletionTimestamp)

		assert.Equal(t, "true", got.Annotations[v1beta1constants.ConfirmationDeletion])
		for _, ann := range cleanupAnnotations {
			assert.Equal(t, "0", got.Annotations[ann], ann)
		}
	})

	t.Run("default", func(t *testing.T) {
		t.Parallel()

		shoot := deletionTestShoot()
		r := deletionTestClient(t, false, shoot)

		require.NoError(t, r.deleteShoot(t.Context(), shoot))

		got := &gardencorev1beta1.Shoot{}
		require.NoError(t, r.client.Get(t.Context(), client.ObjectKeyFromObject(shoot), got))

		assert.Equal(t, "true", got.Annotations[v1beta1constants.ConfirmationDeletion])
		for _, ann := range cleanupAnnotations {
			assert.NotContains(t, got.Annotations, ann)
		}
	})
}
