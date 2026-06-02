package namespace

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/common/namespacename"
)

func newTestHandler(t *testing.T) (*Handler, *shoot.MockShootAccess) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mock := shoot.NewMockShootAccess(logger)
	// queries is intentionally nil: ensure/delete/findByID only touch the shoot
	// client, so these branches can be exercised without a database.
	return &Handler{shoot: mock, logger: logger}, mock
}

func testRow(name string) *db.NamespaceGetForSyncRow {
	return &db.NamespaceGetForSyncRow{
		ID:             uuid.New(),
		ProjectID:      uuid.New(),
		ClusterID:      uuid.New(),
		OrganizationID: uuid.New(),
		ProjectName:    "proj",
		Name:           name,
		ShootStatus:    pgtype.Text{String: "ready", Valid: true},
	}
}

// clusterName is the cluster-side resource name the handler derives for a row.
func clusterName(row *db.NamespaceGetForSyncRow) string {
	return namespacename.Generate(row.ProjectName, row.ProjectID, row.Name)
}

func nsLabels(t *testing.T, mock *shoot.MockShootAccess, clusterID uuid.UUID, name string) map[string]string {
	t.Helper()
	got, err := mock.GetNamespace(context.Background(), clusterID, name)
	require.NoError(t, err)
	require.NotNil(t, got, "namespace %q expected to exist", name)
	return got.Labels
}

func TestDesiredLabels(t *testing.T) {
	t.Parallel()
	row := testRow("team-a")

	labels := desiredLabels(row)

	require.Equal(t, row.ID.String(), labels[LabelNamespaceID])
	require.Equal(t, row.Name, labels[LabelNamespaceName])
	require.Equal(t, row.ProjectID.String(), labels[LabelProjectID])
	require.Equal(t, row.OrganizationID.String(), labels[LabelOrganizationID])
	require.Equal(t, row.ClusterID.String(), labels[LabelClusterID])
	require.Equal(t, ManagedByValue, labels[LabelManagedBy])
}

func TestEnsure_FreshCreate(t *testing.T) {
	t.Parallel()
	h, mock := newTestHandler(t)
	row := testRow("team-a")

	require.NoError(t, h.ensure(context.Background(), row))

	// Created under the project-scoped cluster name, not the bare namespace name.
	labels := nsLabels(t, mock, row.ClusterID, clusterName(row))
	require.Equal(t, row.ID.String(), labels[LabelNamespaceID])
	require.Equal(t, "team-a", labels[LabelNamespaceName])
	require.Equal(t, ManagedByValue, labels[LabelManagedBy])
}

func TestEnsure_IdempotentRecreate(t *testing.T) {
	t.Parallel()
	h, mock := newTestHandler(t)
	row := testRow("team-a")

	require.NoError(t, h.ensure(context.Background(), row))
	require.NoError(t, h.ensure(context.Background(), row), "re-applying must not error")

	labels := nsLabels(t, mock, row.ClusterID, clusterName(row))
	require.Equal(t, row.ID.String(), labels[LabelNamespaceID])
}

func TestEnsure_PatchesDriftedLabelsPreservingOperatorLabels(t *testing.T) {
	t.Parallel()
	h, mock := newTestHandler(t)
	row := testRow("team-a")
	ctx := context.Background()

	// Namespace exists with our id label and an operator-added label, but is
	// missing the rest of the managed set.
	require.NoError(t, mock.CreateNamespace(ctx, row.ClusterID, clusterName(row), map[string]string{
		LabelNamespaceID: row.ID.String(),
		"ops/team":       "platform",
	}))

	require.NoError(t, h.ensure(ctx, row))

	labels := nsLabels(t, mock, row.ClusterID, clusterName(row))
	require.Equal(t, row.ProjectID.String(), labels[LabelProjectID], "missing managed label must be filled in")
	require.Equal(t, ManagedByValue, labels[LabelManagedBy])
	require.Equal(t, "platform", labels["ops/team"], "operator-added labels must be preserved")
}

// If another actor wins the create race between our existence check and Create,
// the conflict must surface as an error so the row retries (and re-runs the
// ownership check) — it must not silently complete and "adopt" the namespace.
func TestEnsure_LostCreateRaceSurfacesError(t *testing.T) {
	t.Parallel()
	h, mock := newTestHandler(t)
	row := testRow("team-a")

	// Simulate the race: the name is absent at Get time, but Create conflicts.
	mock.CreateNamespaceError = apierrors.NewAlreadyExists(corev1.Resource("namespaces"), clusterName(row))

	err := h.ensure(context.Background(), row)
	require.Error(t, err, "a lost create race must not be treated as success")
}

func TestEnsure_NameCollisionReturnsError(t *testing.T) {
	t.Parallel()
	h, mock := newTestHandler(t)
	row := testRow("team-a")
	ctx := context.Background()

	// A namespace with the target cluster name already exists without our label.
	require.NoError(t, mock.CreateNamespace(ctx, row.ClusterID, clusterName(row), map[string]string{"someone": "else"}))

	err := h.ensure(ctx, row)
	require.Error(t, err)
	require.Contains(t, err.Error(), "namespace name collision")
}

// A rename in the DB must not destroy the cluster-side namespace: the immutable
// resource keeps its original name and only the name label is updated.
func TestEnsure_RenameUpdatesLabelKeepsNamespace(t *testing.T) {
	t.Parallel()
	h, mock := newTestHandler(t)
	ctx := context.Background()

	row := testRow("old-name")
	require.NoError(t, h.ensure(ctx, row))
	originalName := clusterName(row)

	// Rename: same id/project, new name. The expected name changes, but the
	// resource must stay put.
	row.Name = "new-name"
	require.NoError(t, h.ensure(ctx, row))

	// No new namespace was created under the renamed expected name.
	renamed, err := mock.GetNamespace(ctx, row.ClusterID, clusterName(row))
	require.NoError(t, err)
	require.Nil(t, renamed, "rename must not create a second namespace")

	// The original namespace still exists and now reflects the new name label.
	labels := nsLabels(t, mock, row.ClusterID, originalName)
	require.Equal(t, row.ID.String(), labels[LabelNamespaceID])
	require.Equal(t, "new-name", labels[LabelNamespaceName], "name label must track the rename")
}

func TestDelete_AlreadyGoneIsNoop(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler(t)
	row := testRow("team-a")

	require.NoError(t, h.delete(context.Background(), row))
}

func TestDelete_MatchingLabelDeletes(t *testing.T) {
	t.Parallel()
	h, mock := newTestHandler(t)
	row := testRow("team-a")
	ctx := context.Background()

	require.NoError(t, mock.CreateNamespace(ctx, row.ClusterID, clusterName(row), map[string]string{
		LabelNamespaceID: row.ID.String(),
	}))

	require.NoError(t, h.delete(ctx, row))

	gone, err := mock.GetNamespace(ctx, row.ClusterID, clusterName(row))
	require.NoError(t, err)
	require.Nil(t, gone)
}

func TestDelete_LabelMismatchIsSkipped(t *testing.T) {
	t.Parallel()
	h, mock := newTestHandler(t)
	row := testRow("team-a")
	ctx := context.Background()

	// Same cluster name, but not ours (different id label).
	require.NoError(t, mock.CreateNamespace(ctx, row.ClusterID, clusterName(row), map[string]string{
		LabelNamespaceID: uuid.New().String(),
	}))

	require.NoError(t, h.delete(ctx, row), "must not error when the namespace is not ours")

	still, err := mock.GetNamespace(ctx, row.ClusterID, clusterName(row))
	require.NoError(t, err)
	require.NotNil(t, still, "a namespace we do not own must not be deleted")
}
