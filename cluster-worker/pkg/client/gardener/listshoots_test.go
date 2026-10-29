package gardener

import (
	"log/slog"
	"testing"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func listShootsTestShoot(name string, labels map[string]string) *gardencorev1beta1.Shoot {
	return &gardencorev1beta1.Shoot{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "garden-local", Labels: labels},
		Spec: gardencorev1beta1.ShootSpec{
			Region:     "local",
			Kubernetes: gardencorev1beta1.Kubernetes{Version: "1.35.5"},
		},
	}
}

// A shoot whose cluster-id label does not parse is still listed, flagged and
// with no cluster id, so reconcile can report it; a shoot without the label is
// not ours and is left out.
func TestListShoots_InvalidClusterIDLabel(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, gardencorev1beta1.AddToScheme(scheme))

	clusterID := uuid.New()
	r := &RealClient{
		client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			listShootsTestShoot("valid", map[string]string{LabelClusterID: clusterID.String()}),
			listShootsTestShoot("invalid", map[string]string{LabelClusterID: "not-a-uuid"}),
			listShootsTestShoot("foreign", nil),
		).Build(),
		provider: NewProviderConfig(),
		logger:   slog.Default(),
	}

	shoots, err := r.ListShoots(t.Context())
	require.NoError(t, err)
	require.Len(t, shoots, 2)

	byName := map[string]ShootInfo{}
	for _, s := range shoots {
		byName[s.Name] = s
	}

	valid := byName["valid"]
	assert.Equal(t, clusterID, valid.ClusterID)
	assert.False(t, valid.InvalidClusterID)

	invalid := byName["invalid"]
	assert.Equal(t, uuid.Nil, invalid.ClusterID)
	assert.True(t, invalid.InvalidClusterID)
}
