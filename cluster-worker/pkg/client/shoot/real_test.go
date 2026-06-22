package shoot

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
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

	err := r.EnsureClusterRoleBinding(context.Background(), uuid.New(), "crb", "ns", "sa",
		map[string]string{"fundament.io/user-id": "u1"},
		map[string]string{"note": "managed"})
	require.NoError(t, err)

	got, err := cs.RbacV1().ClusterRoleBindings().Get(context.Background(), "crb", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, "u1", got.Labels["fundament.io/user-id"])
	require.Equal(t, "managed", got.Annotations["note"])
}
