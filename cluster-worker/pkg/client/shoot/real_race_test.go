package shoot

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// Another writer (a second cluster-worker during a rolling update) ensures the
// same RoleBinding at the same time. Each test makes it win one race window;
// the ensure must start over from the new state instead of failing.

var roleBindingsGVR = schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"}

func raceBinding(role string, uid types.UID) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "rb", Namespace: "ns", UID: uid},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "sa", Namespace: "fundament-system"}},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: role},
	}
}

func ensureRaceBinding(t *testing.T, cs *fake.Clientset, role string) error {
	t.Helper()
	want := raceBinding(role, "")
	return realAccessWith(t, cs).EnsureRoleBinding(context.Background(), uuid.New(), "ns", "rb", want.RoleRef, want.Subjects, nil)
}

func liveRole(t *testing.T, cs *fake.Clientset) string {
	t.Helper()
	rb, err := cs.RbacV1().RoleBindings("ns").Get(context.Background(), "rb", metav1.GetOptions{})
	require.NoError(t, err)
	return rb.RoleRef.Name
}

// once runs react the first time the verb is called and passes later calls on.
func once(cs *fake.Clientset, verb string, react func(k8stesting.Action) (runtime.Object, error)) {
	fired := false
	cs.PrependReactor(verb, "rolebindings", func(a k8stesting.Action) (bool, runtime.Object, error) {
		if fired {
			return false, nil, nil
		}
		fired = true
		obj, err := react(a)
		return true, obj, err
	})
}

func TestEnsureRoleBinding_ObjectDeletedBetweenCreateAndGet(t *testing.T) {
	t.Parallel()
	cs := fake.NewClientset(raceBinding("view", "old"))
	once(cs, "get", func(k8stesting.Action) (runtime.Object, error) {
		require.NoError(t, cs.Tracker().Delete(roleBindingsGVR, "ns", "rb"))
		return nil, apierrors.NewNotFound(roleBindingsGVR.GroupResource(), "rb")
	})

	require.NoError(t, ensureRaceBinding(t, cs, "admin"))
	assert.Equal(t, "admin", liveRole(t, cs))
}

func TestEnsureRoleBinding_RecreatedByAnotherWriter(t *testing.T) {
	t.Parallel()
	cs := fake.NewClientset(raceBinding("view", "old"))
	creates := 0
	cs.PrependReactor("create", "rolebindings", func(k8stesting.Action) (bool, runtime.Object, error) {
		creates++
		if creates != 2 { // 1: initial create (AlreadyExists); 2: our recreate
			return false, nil, nil
		}
		require.NoError(t, cs.Tracker().Add(raceBinding("admin", "theirs")))
		return true, nil, apierrors.NewAlreadyExists(roleBindingsGVR.GroupResource(), "rb")
	})

	require.NoError(t, ensureRaceBinding(t, cs, "admin"))
	assert.Equal(t, "admin", liveRole(t, cs))
}

// The other writer's replacement appears while we wait for our delete: the
// wait must see the new UID as "old one gone" instead of waiting it out.
func TestEnsureRoleBinding_ReplacementDuringDeleteWait(t *testing.T) {
	t.Parallel()
	cs := fake.NewClientset(raceBinding("view", "old"))
	once(cs, "delete", func(k8stesting.Action) (runtime.Object, error) {
		require.NoError(t, cs.Tracker().Delete(roleBindingsGVR, "ns", "rb"))
		require.NoError(t, cs.Tracker().Add(raceBinding("admin", "theirs")))
		return nil, nil
	})

	start := time.Now()
	require.NoError(t, ensureRaceBinding(t, cs, "admin"))
	assert.Less(t, time.Since(start), 5*time.Second, "waited for a deletion that already happened")
	assert.Equal(t, "admin", liveRole(t, cs))
}

func TestEnsureRoleBinding_UpdateConflict(t *testing.T) {
	t.Parallel()
	existing := raceBinding("view", "old")
	existing.Subjects = nil // differs, so the merge updates in place
	cs := fake.NewClientset(existing)
	once(cs, "update", func(k8stesting.Action) (runtime.Object, error) {
		return nil, apierrors.NewConflict(roleBindingsGVR.GroupResource(), "rb", nil)
	})

	require.NoError(t, ensureRaceBinding(t, cs, "view"))
	rb, err := cs.RbacV1().RoleBindings("ns").Get(context.Background(), "rb", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Len(t, rb.Subjects, 1)
}

// A race that keeps happening still fails, after a bounded number of attempts.
func TestEnsureRoleBinding_GivesUpAfterRepeatedRaces(t *testing.T) {
	t.Parallel()
	cs := fake.NewClientset(raceBinding("view", "old"))
	gets := 0
	cs.PrependReactor("get", "rolebindings", func(k8stesting.Action) (bool, runtime.Object, error) {
		gets++
		return true, nil, apierrors.NewNotFound(roleBindingsGVR.GroupResource(), "rb")
	})

	err := ensureRaceBinding(t, cs, "admin")
	require.ErrorIs(t, err, errConcurrentChange)
	assert.Equal(t, ensureAttempts, gets)
}
