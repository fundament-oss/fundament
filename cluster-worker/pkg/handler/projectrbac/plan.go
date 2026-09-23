// Package projectrbac converges the FUN-7 project RoleBindings on a shoot:
// every project member gets a RoleBinding to the built-in admin or view
// ClusterRole in each namespace of their project.
//
// It is not an outbox handler of its own. The registry routes project-member
// rows to usersync and namespace rows to namespace-sync (one default handler
// per entity type), so both call Converger.Converge after their own work, and
// usersync's reconcile loop calls it per ready cluster.
package projectrbac

import (
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
)

const (
	// bindingPrefix names every managed project RoleBinding; the suffix is the
	// user id. A namespace belongs to exactly one project, so one binding per
	// user per namespace suffices.
	bindingPrefix = "fundament:project:"

	// LabelNamespaceID is the ownership label namespace-sync puts on every
	// managed namespace (namespace.LabelNamespaceID; duplicated to avoid an
	// import cycle, pinned equal by a test in that package).
	LabelNamespaceID = "fundament.io/namespace-id"
)

// Desired is one wanted binding: a project member's role in one namespace.
type Desired struct {
	UserID      uuid.UUID
	NamespaceID uuid.UUID
	Role        string // project role: "admin" or "viewer"
}

// Action is one step of a plan. Delete removes the binding; otherwise the
// binding is ensured (created, updated, or recreated on a roleRef change).
type Action struct {
	Delete      bool
	Namespace   string
	Name        string
	UserID      uuid.UUID
	ClusterRole string
}

// BindingName returns the RoleBinding name for a user.
func BindingName(userID uuid.UUID) string {
	return bindingPrefix + userID.String()
}

// ClusterRoleFor maps a project role to the built-in ClusterRole it binds.
// Neither built-in role grants update/patch on namespaces, which keeps the
// fundament.io/project-id label out of tenants' reach (kube-api-proxy filters
// namespace listings on it).
func ClusterRoleFor(projectRole string) string {
	switch projectRole {
	case "admin":
		return "admin"
	case "viewer":
		return "view"
	default:
		panic(fmt.Sprintf("unhandled project role: %s", projectRole))
	}
}

// Subjects returns the subject list of a user's project RoleBinding.
func Subjects(userID uuid.UUID) []rbacv1.Subject {
	return []rbacv1.Subject{{
		Kind:      "ServiceAccount",
		Name:      shoot.SAName(userID),
		Namespace: shoot.FundamentNamespace,
	}}
}

// RoleRef returns the roleRef for a built-in ClusterRole.
func RoleRef(clusterRole string) rbacv1.RoleRef {
	return rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: clusterRole}
}

// Labels returns the labels of a user's project RoleBinding.
func Labels(userID uuid.UUID) map[string]string {
	return map[string]string{shoot.LabelUserID: userID.String()}
}

// BuildPlan diffs the desired bindings against the RoleBindings on the shoot.
// namespaces are the shoot's managed namespaces (carrying LabelNamespaceID);
// desired bindings for namespaces not (yet) on the shoot are skipped — the
// namespace sync converges again once it creates them. actual are RoleBindings
// carrying shoot.LabelUserID; only those named with bindingPrefix are ours.
// It is pure and returns actions in a deterministic order.
func BuildPlan(desired []Desired, namespaces, actual []shoot.ResourceInfo) []Action {
	nsNames := make(map[uuid.UUID]string, len(namespaces))
	for i := range namespaces {
		id, err := uuid.Parse(namespaces[i].Labels[LabelNamespaceID])
		if err != nil {
			continue
		}
		nsNames[id] = namespaces[i].Name
	}

	want := make(map[string]Action, len(desired))
	for _, d := range desired {
		ns, ok := nsNames[d.NamespaceID]
		if !ok {
			continue
		}
		a := Action{Namespace: ns, Name: BindingName(d.UserID), UserID: d.UserID, ClusterRole: ClusterRoleFor(d.Role)}
		want[ns+"/"+a.Name] = a
	}

	var plan []Action
	have := make(map[string]struct{}, len(actual))
	for i := range actual {
		rb := &actual[i]
		if !strings.HasPrefix(rb.Name, bindingPrefix) {
			continue
		}
		key := rb.Namespace + "/" + rb.Name
		have[key] = struct{}{}
		w, ok := want[key]
		if !ok {
			plan = append(plan, Action{Delete: true, Namespace: rb.Namespace, Name: rb.Name})
			continue
		}
		if !matches(rb, w) {
			plan = append(plan, w)
		}
	}
	for key, w := range want {
		if _, ok := have[key]; !ok {
			plan = append(plan, w)
		}
	}

	slices.SortFunc(plan, func(a, b Action) int {
		return strings.Compare(a.Namespace+"/"+a.Name, b.Namespace+"/"+b.Name)
	})
	return plan
}

func matches(rb *shoot.ResourceInfo, w Action) bool {
	return rb.RoleRef == RoleRef(w.ClusterRole) &&
		slices.Equal(rb.Subjects, Subjects(w.UserID)) &&
		rb.Labels[shoot.LabelUserID] == w.UserID.String()
}
