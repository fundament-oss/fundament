package gardener

import (
	"context"
	"io"
	"log/slog"
	"testing"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestClient(t *testing.T, shoots ...*gardencorev1beta1.Shoot) *Client {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, gardencorev1beta1.AddToScheme(scheme))

	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, shoot := range shoots {
		builder = builder.WithObjects(shoot)
	}

	return &Client{
		client: builder.Build(),
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func testShoot(name, clusterID string) *gardencorev1beta1.Shoot {
	return &gardencorev1beta1.Shoot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "garden-local",
			Labels:    map[string]string{LabelClusterID: clusterID},
		},
	}
}

func TestFindShootSingleMatch(t *testing.T) {
	t.Parallel()

	c := newTestClient(t,
		testShoot("shoot-a", "cluster-a"),
		testShoot("shoot-b", "cluster-b"),
	)

	shoot, err := c.FindShoot(context.Background(), "cluster-a")
	require.NoError(t, err)
	assert.Equal(t, "shoot-a", shoot.Name)
}

func TestFindShootNoMatch(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, testShoot("shoot-a", "cluster-a"))

	_, err := c.FindShoot(context.Background(), "cluster-missing")
	require.Error(t, err)
	assert.ErrorContains(t, err, "no shoot found for cluster cluster-missing")
}

// TestFindShootMultipleMatches pins that admin-credentialed traffic is never
// routed to an arbitrary shoot when the cluster-id label is ambiguous.
func TestFindShootMultipleMatches(t *testing.T) {
	t.Parallel()

	c := newTestClient(t,
		testShoot("shoot-a", "cluster-a"),
		testShoot("shoot-a-clone", "cluster-a"),
	)

	_, err := c.FindShoot(context.Background(), "cluster-a")
	require.Error(t, err)
	assert.ErrorContains(t, err, "found 2 shoots labeled with cluster cluster-a")
}
