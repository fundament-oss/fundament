package namespace

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
)

func newTestHandler(t *testing.T) (*Handler, *shoot.MockShootAccess) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mock := shoot.NewMockShootAccess(logger)
	// queries is intentionally nil: ensure/delete/deleteRenamed only touch the
	// shoot client, so these branches can be exercised without a database.
	return &Handler{shoot: mock, logger: logger}, mock
}

func testRow(name string) *db.NamespaceGetForSyncRow {
	return &db.NamespaceGetForSyncRow{
		ID:             uuid.New(),
		ProjectID:      uuid.New(),
		ClusterID:      uuid.New(),
		OrganizationID: uuid.New(),
		Name:           name,
		ShootStatus:    pgtype.Text{String: "ready", Valid: true},
	}
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

	labels := nsLabels(t, mock, row.ClusterID, "team-a")
	require.Equal(t, row.ID.String(), labels[LabelNamespaceID])
	require.Equal(t, ManagedByValue, labels[LabelManagedBy])
}

func TestEnsure_IdempotentRecreate(t *testing.T) {
	t.Parallel()
	h, mock := newTestHandler(t)
	row := testRow("team-a")

	require.NoError(t, h.ensure(context.Background(), row))
	require.NoError(t, h.ensure(context.Background(), row), "re-applying must not error")

	labels := nsLabels(t, mock, row.ClusterID, "team-a")
	require.Equal(t, row.ID.String(), labels[LabelNamespaceID])
}

func TestEnsure_PatchesDriftedLabelsPreservingOperatorLabels(t *testing.T) {
	t.Parallel()
	h, mock := newTestHandler(t)
	row := testRow("team-a")
	ctx := context.Background()

	// Namespace exists with our id label and an operator-added label, but is
	// missing the rest of the managed set.
	require.NoError(t, mock.CreateNamespace(ctx, row.ClusterID, "team-a", map[string]string{
		LabelNamespaceID: row.ID.String(),
		"ops/team":       "platform",
	}))

	require.NoError(t, h.ensure(ctx, row))

	labels := nsLabels(t, mock, row.ClusterID, "team-a")
	require.Equal(t, row.ProjectID.String(), labels[LabelProjectID], "missing managed label must be filled in")
	require.Equal(t, ManagedByValue, labels[LabelManagedBy])
	require.Equal(t, "platform", labels["ops/team"], "operator-added labels must be preserved")
}

func TestEnsure_NameCollisionReturnsError(t *testing.T) {
	t.Parallel()
	h, mock := newTestHandler(t)
	row := testRow("team-a")
	ctx := context.Background()

	// A namespace with the target name already exists without our label.
	require.NoError(t, mock.CreateNamespace(ctx, row.ClusterID, "team-a", map[string]string{"someone": "else"}))

	err := h.ensure(ctx, row)
	require.Error(t, err)
	require.Contains(t, err.Error(), "namespace name collision: team-a already exists on shoot without matching label")
}

func TestEnsure_RenameDeletesOldCreatesNew(t *testing.T) {
	t.Parallel()
	h, mock := newTestHandler(t)
	row := testRow("new-name")
	ctx := context.Background()

	// An older cluster-side namespace carries this row's id under the old name.
	require.NoError(t, mock.CreateNamespace(ctx, row.ClusterID, "old-name", map[string]string{
		LabelNamespaceID: row.ID.String(),
	}))

	require.NoError(t, h.ensure(ctx, row))

	gone, err := mock.GetNamespace(ctx, row.ClusterID, "old-name")
	require.NoError(t, err)
	require.Nil(t, gone, "old-name namespace must be deleted on rename")

	labels := nsLabels(t, mock, row.ClusterID, "new-name")
	require.Equal(t, row.ID.String(), labels[LabelNamespaceID])
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

	require.NoError(t, mock.CreateNamespace(ctx, row.ClusterID, "team-a", map[string]string{
		LabelNamespaceID: row.ID.String(),
	}))

	require.NoError(t, h.delete(ctx, row))

	gone, err := mock.GetNamespace(ctx, row.ClusterID, "team-a")
	require.NoError(t, err)
	require.Nil(t, gone)
}

func TestDelete_LabelMismatchIsSkipped(t *testing.T) {
	t.Parallel()
	h, mock := newTestHandler(t)
	row := testRow("team-a")
	ctx := context.Background()

	// Same name, but not ours (different id label).
	require.NoError(t, mock.CreateNamespace(ctx, row.ClusterID, "team-a", map[string]string{
		LabelNamespaceID: uuid.New().String(),
	}))

	require.NoError(t, h.delete(ctx, row), "must not error when the namespace is not ours")

	still, err := mock.GetNamespace(ctx, row.ClusterID, "team-a")
	require.NoError(t, err)
	require.NotNil(t, still, "a namespace we do not own must not be deleted")
}
