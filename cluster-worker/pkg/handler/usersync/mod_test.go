package usersync

import (
	"testing"

	"github.com/google/uuid"
	rbacv1 "k8s.io/api/rbac/v1"
)

func TestGroupResourcesByUserIDSeparatesOrphans(t *testing.T) {
	userID := uuid.New()
	resources := []ResourceInfo{
		{Name: SAName(userID), Labels: map[string]string{LabelUserID: userID.String()}},
		{Name: "orphan-missing-label"},
		{Name: "orphan-invalid-label", Labels: map[string]string{LabelUserID: "not-a-uuid"}},
	}

	grouped, orphans := groupResourcesByUserID(resources)

	if len(grouped[userID]) != 1 {
		t.Fatalf("expected 1 grouped resource for user, got %d", len(grouped[userID]))
	}
	if _, ok := orphans["orphan-missing-label"]; !ok {
		t.Fatalf("expected missing-label resource to be orphaned")
	}
	if _, ok := orphans["orphan-invalid-label"]; !ok {
		t.Fatalf("expected invalid-label resource to be orphaned")
	}
}

func TestClassifyServiceAccountsDetectsDuplicates(t *testing.T) {
	userID := uuid.New()
	labels := map[string]string{LabelUserID: userID.String()}
	annotations := map[string]string{AnnotationUserName: "user@example.com"}

	resources := []ResourceInfo{
		{
			Name:        SAName(userID),
			Labels:      cloneStringMap(labels),
			Annotations: cloneStringMap(annotations),
		},
		{
			Name:        "fundament-duplicate",
			Labels:      cloneStringMap(labels),
			Annotations: cloneStringMap(annotations),
		},
	}

	hasCanonical, duplicates := classifyServiceAccounts(resources, userID, labels, annotations)

	if !hasCanonical {
		t.Fatalf("expected canonical service account to be healthy")
	}
	if len(duplicates) != 1 || duplicates[0] != "fundament-duplicate" {
		t.Fatalf("expected duplicate service account to be reported, got %v", duplicates)
	}
}

func TestClassifyClusterRoleBindingsDetectsDriftAndDuplicates(t *testing.T) {
	userID := uuid.New()
	labels := map[string]string{LabelUserID: userID.String()}
	annotations := map[string]string{AnnotationUserName: "user@example.com"}

	resources := []ResourceInfo{
		{
			Name:        CRBName(userID),
			Labels:      cloneStringMap(labels),
			Annotations: cloneStringMap(annotations),
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "ClusterRole",
				Name:     "view",
			},
			Subjects: []rbacv1.Subject{{
				Kind:      "ServiceAccount",
				Name:      SAName(userID),
				Namespace: FundamentNamespace,
			}},
		},
		{
			Name:        "fundament:admin:duplicate",
			Labels:      cloneStringMap(labels),
			Annotations: cloneStringMap(annotations),
		},
	}

	hasCanonical, duplicates := classifyClusterRoleBindings(resources, userID, labels, annotations)

	if hasCanonical {
		t.Fatalf("expected drifted canonical cluster role binding to be unhealthy")
	}
	if len(duplicates) != 1 || duplicates[0] != "fundament:admin:duplicate" {
		t.Fatalf("expected duplicate cluster role binding to be reported, got %v", duplicates)
	}
}

func TestClusterRoleBindingNeedsRecreateWhenRoleRefDrifts(t *testing.T) {
	existing := &rbacv1.ClusterRoleBinding{
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "view",
		},
	}
	desired := &rbacv1.ClusterRoleBinding{
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
	}

	if !clusterRoleBindingNeedsRecreate(existing, desired) {
		t.Fatalf("expected changed roleRef to require recreate")
	}
}
