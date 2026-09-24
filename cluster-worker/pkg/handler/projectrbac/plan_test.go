package projectrbac

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
)

var (
	userA = uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	userB = uuid.MustParse("00000000-0000-0000-0000-00000000000b")
	nsWeb = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	nsAPI = uuid.MustParse("00000000-0000-0000-0000-000000000002")
)

func managedNamespace(name string, id uuid.UUID) shoot.ResourceInfo {
	return shoot.ResourceInfo{Name: name, Labels: map[string]string{LabelNamespaceID: id.String()}}
}

func binding(ns string, user uuid.UUID, clusterRole string) shoot.ResourceInfo {
	return shoot.ResourceInfo{
		Name:      BindingName(user),
		Namespace: ns,
		Labels:    Labels(user),
		RoleRef:   RoleRef(clusterRole),
		Subjects:  Subjects(user),
	}
}

func ensure(ns string, user uuid.UUID, clusterRole string) Action {
	return Action{Namespace: ns, Name: BindingName(user), UserID: user, ClusterRole: clusterRole}
}

func remove(ns string, user uuid.UUID) Action {
	return Action{Delete: true, Namespace: ns, Name: BindingName(user)}
}

func TestBuildPlan(t *testing.T) {
	t.Parallel()

	namespaces := []shoot.ResourceInfo{managedNamespace("p--web", nsWeb), managedNamespace("p--api", nsAPI)}

	tests := []struct {
		name       string
		desired    []Desired
		namespaces []shoot.ResourceInfo
		actual     []shoot.ResourceInfo
		want       []Action
	}{
		{
			name:       "member added gets a binding per project namespace",
			desired:    []Desired{{userA, nsWeb, "viewer"}, {userA, nsAPI, "viewer"}},
			namespaces: namespaces,
			want:       []Action{ensure("p--api", userA, "view"), ensure("p--web", userA, "view")},
		},
		{
			name:       "project admin binds the admin ClusterRole",
			desired:    []Desired{{userA, nsWeb, "admin"}},
			namespaces: namespaces,
			want:       []Action{ensure("p--web", userA, "admin")},
		},
		{
			name:       "converged state is a no-op",
			desired:    []Desired{{userA, nsWeb, "viewer"}},
			namespaces: namespaces,
			actual:     []shoot.ResourceInfo{binding("p--web", userA, "view")},
			want:       nil,
		},
		{
			name:       "promotion re-ensures with the new role",
			desired:    []Desired{{userA, nsWeb, "admin"}},
			namespaces: namespaces,
			actual:     []shoot.ResourceInfo{binding("p--web", userA, "view")},
			want:       []Action{ensure("p--web", userA, "admin")},
		},
		{
			name:       "removed member's bindings are deleted",
			desired:    []Desired{{userB, nsWeb, "viewer"}},
			namespaces: namespaces,
			actual:     []shoot.ResourceInfo{binding("p--web", userA, "view"), binding("p--web", userB, "view")},
			want:       []Action{remove("p--web", userA)},
		},
		{
			name:       "namespace not yet on the shoot is skipped",
			desired:    []Desired{{userA, nsWeb, "viewer"}, {userA, nsAPI, "viewer"}},
			namespaces: []shoot.ResourceInfo{managedNamespace("p--web", nsWeb)},
			want:       []Action{ensure("p--web", userA, "view")},
		},
		{
			name:       "binding in a namespace that left the desired set is deleted",
			desired:    nil,
			namespaces: namespaces,
			actual:     []shoot.ResourceInfo{binding("p--api", userA, "admin")},
			want:       []Action{remove("p--api", userA)},
		},
		{
			name:       "drifted subjects are re-ensured",
			desired:    []Desired{{userA, nsWeb, "viewer"}},
			namespaces: namespaces,
			actual: []shoot.ResourceInfo{func() shoot.ResourceInfo {
				rb := binding("p--web", userA, "view")
				rb.Subjects = []rbacv1.Subject{{Kind: "ServiceAccount", Name: "intruder", Namespace: "default"}}
				return rb
			}()},
			want: []Action{ensure("p--web", userA, "view")},
		},
		{
			name:       "foreign bindings with the user label are left alone",
			desired:    nil,
			namespaces: namespaces,
			actual: []shoot.ResourceInfo{{
				Name: "team-edit", Namespace: "p--web", Labels: Labels(userA), RoleRef: RoleRef("edit"),
			}},
			want: nil,
		},
		{
			name:       "namespaces with a malformed id label are ignored",
			desired:    []Desired{{userA, nsWeb, "viewer"}},
			namespaces: []shoot.ResourceInfo{{Name: "p--web", Labels: map[string]string{LabelNamespaceID: "not-a-uuid"}}},
			want:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, BuildPlan(tt.desired, tt.namespaces, tt.actual))
		})
	}
}

func TestClusterRoleFor(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "admin", ClusterRoleFor("admin"))
	assert.Equal(t, "view", ClusterRoleFor("viewer"))
	assert.Panics(t, func() { ClusterRoleFor("owner") })
}
