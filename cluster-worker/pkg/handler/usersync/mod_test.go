package usersync

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
)

func newTestShootAccess(t *testing.T) *MockShootAccess {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewMockShootAccess(logger)
}

func TestApplyUserAccessAdmin(t *testing.T) {
	t.Parallel()

	mock := newTestShootAccess(t)
	h := &Handler{shoot: mock, logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))}

	clusterID := uuid.New()
	userID := uuid.New()

	err := h.applyUserAccess(context.Background(), clusterID, userID, "admin@example.com", "admin")
	require.NoError(t, err)

	require.True(t, mock.HasSA(clusterID, userID), "SA should exist")
	require.True(t, mock.HasCRB(clusterID, userID), "CRB should exist")
}

func TestApplyUserAccessMember(t *testing.T) {
	t.Parallel()

	mock := newTestShootAccess(t)
	h := &Handler{shoot: mock, logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))}

	clusterID := uuid.New()
	userID := uuid.New()

	err := h.applyUserAccess(context.Background(), clusterID, userID, "member@example.com", "member")
	require.NoError(t, err)

	require.True(t, mock.HasSA(clusterID, userID), "SA should exist")
	require.False(t, mock.HasCRB(clusterID, userID), "CRB should not exist for member")
}

func TestApplyUserAccessNone(t *testing.T) {
	t.Parallel()

	mock := newTestShootAccess(t)
	h := &Handler{shoot: mock, logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))}

	clusterID := uuid.New()
	userID := uuid.New()

	// Pre-populate SA + CRB to verify they get deleted.
	_ = mock.EnsureServiceAccount(context.Background(), clusterID, FundamentNamespace, SAName(userID), nil, nil)
	_ = mock.EnsureClusterRoleBinding(context.Background(), clusterID, CRBName(userID), "", "", nil, nil)
	require.True(t, mock.HasSA(clusterID, userID))
	require.True(t, mock.HasCRB(clusterID, userID))

	err := h.applyUserAccess(context.Background(), clusterID, userID, "removed@example.com", "none")
	require.NoError(t, err)

	require.False(t, mock.HasSA(clusterID, userID), "SA should be deleted")
	require.False(t, mock.HasCRB(clusterID, userID), "CRB should be deleted")
}

func TestApplyUserAccessIdempotent(t *testing.T) {
	t.Parallel()

	mock := newTestShootAccess(t)
	h := &Handler{shoot: mock, logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))}

	clusterID := uuid.New()
	userID := uuid.New()

	// Apply admin twice — should succeed both times with same result.
	err := h.applyUserAccess(context.Background(), clusterID, userID, "admin@example.com", "admin")
	require.NoError(t, err)

	err = h.applyUserAccess(context.Background(), clusterID, userID, "admin@example.com", "admin")
	require.NoError(t, err)

	require.True(t, mock.HasSA(clusterID, userID))
	require.True(t, mock.HasCRB(clusterID, userID))
}

func TestApplyUserAccessDemotionAdminToMember(t *testing.T) {
	t.Parallel()

	mock := newTestShootAccess(t)
	h := &Handler{shoot: mock, logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))}

	clusterID := uuid.New()
	userID := uuid.New()

	// Start as admin.
	err := h.applyUserAccess(context.Background(), clusterID, userID, "user@example.com", "admin")
	require.NoError(t, err)
	require.True(t, mock.HasSA(clusterID, userID))
	require.True(t, mock.HasCRB(clusterID, userID))

	// Demote to member.
	err = h.applyUserAccess(context.Background(), clusterID, userID, "user@example.com", "member")
	require.NoError(t, err)
	require.True(t, mock.HasSA(clusterID, userID), "SA should be kept")
	require.False(t, mock.HasCRB(clusterID, userID), "CRB should be deleted")
}

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
