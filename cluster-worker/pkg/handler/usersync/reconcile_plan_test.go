package usersync

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
)

func TestBuildReconcilePlan_MissingSA(t *testing.T) {
	userID := uuid.New()
	desired := []db.UserListForClusterRow{
		{UserID: userID, Email: pgtype.Text{String: "user@example.com", Valid: true}, AccessLevel: "admin"},
	}
	plan := buildReconcilePlan(desired, nil, nil)

	var hasEnsureSA, hasEnsureCRB bool
	for _, a := range plan {
		if a.Type == ActionEnsureSA && a.UserID == userID {
			hasEnsureSA = true
		}
		if a.Type == ActionEnsureCRB && a.UserID == userID {
			hasEnsureCRB = true
		}
	}
	if !hasEnsureSA {
		t.Error("expected EnsureSA action for missing SA")
	}
	if !hasEnsureCRB {
		t.Error("expected EnsureCRB action for missing CRB")
	}
}

func TestBuildReconcilePlan_OrphanedSA(t *testing.T) {
	orphanID := uuid.New()
	actualSAs := []shoot.ResourceInfo{
		{
			Name:   shoot.SAName(orphanID),
			Labels: map[string]string{shoot.LabelUserID: orphanID.String()},
		},
	}
	plan := buildReconcilePlan(nil, actualSAs, nil)

	var hasDeleteSA bool
	for _, a := range plan {
		if a.Type == ActionDeleteSA && a.Name == shoot.SAName(orphanID) {
			hasDeleteSA = true
		}
	}
	if !hasDeleteSA {
		t.Error("expected DeleteSA action for orphaned SA")
	}
}

func TestBuildReconcilePlan_CRBMismatch(t *testing.T) {
	userID := uuid.New()
	desired := []db.UserListForClusterRow{
		{UserID: userID, Email: pgtype.Text{String: "user@example.com", Valid: true}, AccessLevel: "member"},
	}

	// SA is healthy
	actualSAs := []shoot.ResourceInfo{
		{
			Name:        shoot.SAName(userID),
			Labels:      map[string]string{shoot.LabelUserID: userID.String()},
			Annotations: map[string]string{shoot.AnnotationUserName: "user@example.com"},
		},
	}

	// CRB exists but user is member — should be deleted
	actualCRBs := []shoot.ResourceInfo{
		{
			Name:   shoot.CRBName(userID),
			Labels: map[string]string{shoot.LabelUserID: userID.String()},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "ClusterRole",
				Name:     "cluster-admin",
			},
			Subjects: []rbacv1.Subject{{
				Kind:      "ServiceAccount",
				Name:      shoot.SAName(userID),
				Namespace: shoot.FundamentNamespace,
			}},
		},
	}

	plan := buildReconcilePlan(desired, actualSAs, actualCRBs)

	var hasDeleteCRB bool
	for _, a := range plan {
		if a.Type == ActionDeleteCRB && a.Name == shoot.CRBName(userID) {
			hasDeleteCRB = true
		}
	}
	if !hasDeleteCRB {
		t.Error("expected DeleteCRB action for member with stale CRB")
	}
}

func TestBuildReconcilePlan_Duplicates(t *testing.T) {
	userID := uuid.New()
	desired := []db.UserListForClusterRow{
		{UserID: userID, Email: pgtype.Text{String: "user@example.com", Valid: true}, AccessLevel: "admin"},
	}

	labels := map[string]string{shoot.LabelUserID: userID.String()}
	annotations := map[string]string{shoot.AnnotationUserName: "user@example.com"}

	actualSAs := []shoot.ResourceInfo{
		{Name: shoot.SAName(userID), Labels: shoot.CloneStringMap(labels), Annotations: shoot.CloneStringMap(annotations)},
		{Name: "fundament-duplicate", Labels: shoot.CloneStringMap(labels), Annotations: shoot.CloneStringMap(annotations)},
	}
	actualCRBs := []shoot.ResourceInfo{
		{
			Name:        shoot.CRBName(userID),
			Labels:      shoot.CloneStringMap(labels),
			Annotations: shoot.CloneStringMap(annotations),
			RoleRef:     rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: "cluster-admin"},
			Subjects:    []rbacv1.Subject{{Kind: "ServiceAccount", Name: shoot.SAName(userID), Namespace: shoot.FundamentNamespace}},
		},
	}

	plan := buildReconcilePlan(desired, actualSAs, actualCRBs)

	var deleteSACount int
	for _, a := range plan {
		if a.Type == ActionDeleteSA {
			deleteSACount++
		}
	}
	if deleteSACount != 1 {
		t.Errorf("expected 1 DeleteSA action for duplicate, got %d", deleteSACount)
	}
}

func TestBuildReconcilePlan_HealthyStateNoActions(t *testing.T) {
	userID := uuid.New()
	desired := []db.UserListForClusterRow{
		{UserID: userID, Email: pgtype.Text{String: "user@example.com", Valid: true}, AccessLevel: "admin"},
	}

	labels := map[string]string{shoot.LabelUserID: userID.String()}
	annotations := map[string]string{shoot.AnnotationUserName: "user@example.com"}

	actualSAs := []shoot.ResourceInfo{
		{Name: shoot.SAName(userID), Labels: shoot.CloneStringMap(labels), Annotations: shoot.CloneStringMap(annotations)},
	}
	actualCRBs := []shoot.ResourceInfo{
		{
			Name:        shoot.CRBName(userID),
			Labels:      shoot.CloneStringMap(labels),
			Annotations: shoot.CloneStringMap(annotations),
			RoleRef:     rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: "cluster-admin"},
			Subjects:    []rbacv1.Subject{{Kind: "ServiceAccount", Name: shoot.SAName(userID), Namespace: shoot.FundamentNamespace}},
		},
	}

	plan := buildReconcilePlan(desired, actualSAs, actualCRBs)

	if len(plan) != 0 {
		t.Errorf("expected empty plan for healthy state, got %d actions", len(plan))
	}
}
