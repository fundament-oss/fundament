package shoot

import (
	"context"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// Namespaces go to the sandbox cluster, where a plugin can use them; the rest
// of the shoot state stays in memory.
func TestSandboxShootAccess_NamespacesOnSandbox(t *testing.T) {
	t.Parallel()
	cs := fake.NewClientset()
	s := &SandboxShootAccess{
		MockShootAccess: NewMockShootAccess(slog.Default()),
		sandbox:         realAccessWith(t, cs),
	}
	ctx := context.Background()
	clusterID := uuid.New()

	require.NoError(t, s.CreateNamespace(ctx, clusterID, "tnt-p--web", map[string]string{"fundament.io/namespace-id": "n1"}))
	got, err := cs.CoreV1().Namespaces().Get(ctx, "tnt-p--web", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "n1", got.Labels["fundament.io/namespace-id"])
	assert.Empty(t, s.MockShootAccess.Namespaces[clusterID], "the namespace is not kept in memory")

	found, err := s.FindNamespaceByLabel(ctx, clusterID, "fundament.io/namespace-id", "n1")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "tnt-p--web", found.Name)

	require.NoError(t, s.EnsureLimitRange(ctx, clusterID, "tnt-p--web", LimitDefaults{}, nil))
	lrs, err := cs.CoreV1().LimitRanges("tnt-p--web").List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	assert.Empty(t, lrs.Items, "LimitRanges stay in memory")
}
