// Package namespace materializes tenant.namespaces rows as v1/Namespace
// resources on their owning shoot cluster. It mirrors, and is disjoint from,
// the usersync handler: usersync manages ServiceAccounts/ClusterRoleBindings
// inside fundament-system, whereas namespace-sync manages the Namespace
// resource itself. Soft-deletion in the fundament DB hard-deletes the
// cluster-side namespace, exactly as cluster-user-sync treats soft-deletes.
package namespace

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	"github.com/fundament-oss/fundament/common/kubename"
)

// Label keys applied to every fundament-managed namespace on a shoot. The
// fundament.io/ prefix matches the existing convention (usersync uses
// fundament.io/user-id). LabelNamespaceID is the canonical ownership marker:
// reconcile and delete only ever touch namespaces carrying it. LabelNamespaceName
// records the fundament-side name as informational metadata so operators can
// correlate the resource back to fundament without parsing the generated name.
const (
	LabelNamespaceID    = "fundament.io/namespace-id"
	LabelNamespaceName  = "fundament.io/namespace-name"
	LabelProjectID      = "fundament.io/project-id"
	LabelOrganizationID = "fundament.io/organization-id"
	LabelClusterID      = "fundament.io/cluster-id"
	LabelManagedBy      = "fundament.io/managed-by"

	// ManagedByValue is the value of LabelManagedBy on managed namespaces.
	ManagedByValue = "cluster-worker"
)

// Handler reconciles tenant.namespaces rows to v1/Namespace resources on the
// owning shoot cluster.
type Handler struct {
	pool       *pgxpool.Pool
	queries    *db.Queries
	shoot      shoot.ShootAccess
	maxRetries int32
	logger     *slog.Logger
}

// New constructs a namespace handler. maxRetries mirrors the outbox worker's
// MaxRetries so reconcile's conditional enqueue uses the same exhaustion
// threshold as cluster reconcile.
func New(pool *pgxpool.Pool, shootAccess shoot.ShootAccess, maxRetries int32, logger *slog.Logger) *Handler {
	return &Handler{
		pool:       pool,
		queries:    db.New(pool),
		shoot:      shootAccess,
		maxRetries: maxRetries,
		logger:     logger.With("handler", "namespace"),
	}
}

// Sync dispatches an outbox row to the right handler. Namespace rows converge a
// single namespace; the cluster-ready event fans out a sync for every active
// namespace on that cluster (the handler subscribes to the ready event directly,
// so the cluster status handler stays free of namespace concerns).
func (h *Handler) Sync(ctx context.Context, id uuid.UUID, sc handler.SyncContext) error {
	switch sc.EntityType {
	case handler.EntityNamespace:
		return h.syncNamespace(ctx, id)
	case handler.EntityCluster:
		// id is the cluster id for the ready event.
		return h.enqueueClusterNamespaces(ctx, id)
	default:
		return fmt.Errorf("unexpected entity type %s for namespace sync handler", sc.EntityType)
	}
}

// syncNamespace converges a single tenant.namespaces row to its cluster-side
// v1/Namespace. It is idempotent and event-agnostic: it reloads the row and
// reconciles to the desired state, so created/updated/deleted/reconcile rows
// all resolve correctly without branching on the event.
func (h *Handler) syncNamespace(ctx context.Context, id uuid.UUID) error {
	row, err := h.queries.NamespaceGetForSync(ctx, db.NamespaceGetForSyncParams{ID: id})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.logger.Info("namespace not found, skipping", "namespace_id", id)
			return nil
		}
		return fmt.Errorf("get namespace for sync: %w", err)
	}

	// Gate on shoot readiness — defer (don't fail) until the shoot is ready.
	// The cluster-ready fan-out re-enqueues these rows once the shoot is up.
	if !row.ShootStatus.Valid || row.ShootStatus.String != string(gardener.StatusReady) {
		return handler.NewPreconditionError("shoot not ready")
	}

	if row.Deleted.Valid {
		return h.delete(ctx, &row)
	}
	return h.ensure(ctx, &row)
}

// ensure creates or label-reconciles the cluster-side namespace for an active row.
// The cluster-side resource name is derived from the project and the namespace
// name (kubename.GenerateNamespace) so two projects on the same shoot never collide.
// Ownership is tracked by the LabelNamespaceID label. The name derives only from
// immutable inputs (project id, project name, and the namespace name — none of
// which can change), so the expected name is stable for the life of the row and a
// single lookup by that name is authoritative.
func (h *Handler) ensure(ctx context.Context, row *db.NamespaceGetForSyncRow) error {
	desired := desiredLabels(row)
	name := kubename.GenerateNamespace(row.ProjectName, row.ProjectID, row.Name)

	existing, err := h.shoot.GetNamespace(ctx, row.ClusterID, name)
	if err != nil {
		return fmt.Errorf("get namespace %s: %w", name, err)
	}
	if existing != nil {
		// Ours: reconcile labels. Not ours: defer if it's a managed sibling still
		// being cleaned up, else report a real collision.
		if existing.Labels[LabelNamespaceID] == row.ID.String() {
			return h.reconcileLabels(ctx, row, name, desired)
		}
		return h.nameTakenError(name, existing)
	}

	if err := h.shoot.CreateNamespace(ctx, row.ClusterID, name, desired); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create namespace %s: %w", name, err)
		}
		// Lost a create race between the existence check above and Create — most
		// plausibly a duplicate reconcile row processed concurrently. Re-read and
		// converge instead of failing the row: adopt it if it is now ours, defer if
		// it is a managed sibling mid-cleanup, or report a real collision.
		raced, gerr := h.shoot.GetNamespace(ctx, row.ClusterID, name)
		if gerr != nil {
			return fmt.Errorf("get namespace %s after create conflict: %w", name, gerr)
		}
		if raced == nil {
			return handler.NewPreconditionError(fmt.Sprintf("namespace %s create raced, retrying", name))
		}
		if raced.Labels[LabelNamespaceID] == row.ID.String() {
			return h.reconcileLabels(ctx, row, name, desired)
		}
		return h.nameTakenError(name, raced)
	}
	h.logger.Info("created namespace",
		"namespace_id", row.ID, "cluster_id", row.ClusterID, "name", name)
	return nil
}

// nameTakenError classifies an existing same-named namespace that is not ours by
// id. If it carries our managed-by marker it is a same-named sibling still being
// cleaned up: the cluster-side name omits the row id, so a soft-delete + recreate
// of the same name maps to the same name and the old namespace lingers until its
// delete row processes (or it finishes Terminating). Defer in that case so we
// don't burn retries toward the failed state — the sibling's removal frees the
// name and the row converges. A genuinely foreign namespace (system/operator) is
// a real collision we must never adopt.
func (h *Handler) nameTakenError(name string, existing *shoot.ResourceInfo) error {
	if existing.Labels[LabelManagedBy] == ManagedByValue {
		return handler.NewPreconditionError(
			fmt.Sprintf("namespace %s held by another managed namespace, awaiting cleanup", name))
	}
	return fmt.Errorf("namespace name collision: %s already exists on shoot without matching label", name)
}

// reconcileLabels merges the desired managed labels onto the existing namespace,
// preserving any operator-added labels.
func (h *Handler) reconcileLabels(ctx context.Context, row *db.NamespaceGetForSyncRow, name string, desired map[string]string) error {
	if err := h.shoot.UpdateNamespaceLabels(ctx, row.ClusterID, name, desired); err != nil {
		return fmt.Errorf("update namespace %s labels: %w", name, err)
	}
	h.logger.Debug("namespace present, labels reconciled",
		"namespace_id", row.ID, "cluster_id", row.ClusterID, "name", name)
	return nil
}

// delete hard-deletes the cluster-side namespace for a soft-deleted row. The
// expected name is stable (derived from immutable inputs), so a single lookup is
// authoritative: a namespace that is absent, or present without our id label, is
// never touched (idempotent).
func (h *Handler) delete(ctx context.Context, row *db.NamespaceGetForSyncRow) error {
	name := kubename.GenerateNamespace(row.ProjectName, row.ProjectID, row.Name)

	existing, err := h.shoot.GetNamespace(ctx, row.ClusterID, name)
	if err != nil {
		return fmt.Errorf("get namespace %s: %w", name, err)
	}
	if existing == nil || existing.Labels[LabelNamespaceID] != row.ID.String() {
		return nil // already gone, or not ours — idempotent
	}

	if err := h.shoot.DeleteNamespace(ctx, row.ClusterID, name); err != nil {
		return fmt.Errorf("delete namespace %s: %w", name, err)
	}
	h.logger.Info("deleted namespace",
		"namespace_id", row.ID, "cluster_id", row.ClusterID, "name", name)
	return nil
}

// enqueueClusterNamespaces fans out a sync for every active namespace on a
// cluster that just became ready. Namespaces created while the shoot was still
// provisioning materialize once they are picked up again. The conditional
// insert makes this safe to call repeatedly. Mirrors how usersync reacts to the
// same cluster-ready event by provisioning all of the cluster's users.
func (h *Handler) enqueueClusterNamespaces(ctx context.Context, clusterID uuid.UUID) error {
	namespaceIDs, err := h.queries.NamespaceListActiveForCluster(ctx, db.NamespaceListActiveForClusterParams{
		ClusterID: clusterID,
	})
	if err != nil {
		return fmt.Errorf("list active namespaces: %w", err)
	}

	var errs []error
	for _, nsID := range namespaceIDs {
		if err := h.queries.OutboxInsertReconcileForNamespace(ctx, db.OutboxInsertReconcileForNamespaceParams{
			NamespaceID: pgtype.UUID{Bytes: nsID, Valid: true},
			MaxRetries:  h.maxRetries,
		}); err != nil {
			errs = append(errs, fmt.Errorf("enqueue namespace %s: %w", nsID, err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("enqueue cluster namespaces for %s: %w", clusterID, err)
	}
	return nil
}

// Reconcile performs full namespace reconciliation for every ready cluster:
// enqueue sync for namespaces missing on the shoot, hard-delete orphans.
func (h *Handler) Reconcile(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil //nolint:nilerr // graceful shutdown
	}

	h.logger.Info("starting namespace sync reconciliation")

	clusterIDs, err := h.queries.ClusterListReady(ctx)
	if err != nil {
		return fmt.Errorf("list ready clusters: %w", err)
	}

	var errs []error
	for _, clusterID := range clusterIDs {
		if ctx.Err() != nil {
			return nil //nolint:nilerr // graceful shutdown
		}
		if err := h.reconcileCluster(ctx, clusterID); err != nil {
			h.logger.Error("failed to reconcile namespaces for cluster",
				"cluster_id", clusterID, "error", err)
			errs = append(errs, err)
		}
	}

	h.logger.Info("namespace sync reconciliation complete", "clusters", len(clusterIDs))
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("namespace sync reconcile: %w", err)
	}
	return nil
}

func (h *Handler) reconcileCluster(ctx context.Context, clusterID uuid.UUID) error {
	activeIDs, err := h.queries.NamespaceListActiveForCluster(ctx, db.NamespaceListActiveForClusterParams{ClusterID: clusterID})
	if err != nil {
		return fmt.Errorf("list active namespaces: %w", err)
	}

	clusterNamespaces, err := h.shoot.ListNamespaces(ctx, clusterID, LabelNamespaceID)
	if err != nil {
		return fmt.Errorf("list cluster namespaces: %w", err)
	}

	plan := BuildPlan(activeIDs, clusterNamespaces)

	var errs []error
	for _, id := range plan.CreateIDs {
		if err := h.queries.OutboxInsertReconcileForNamespace(ctx, db.OutboxInsertReconcileForNamespaceParams{
			NamespaceID: pgtype.UUID{Bytes: id, Valid: true},
			MaxRetries:  h.maxRetries,
		}); err != nil {
			errs = append(errs, fmt.Errorf("enqueue namespace %s: %w", id, err))
		}
	}
	// Re-check ownership against live DB state at delete time, not the snapshot
	// above. The cluster-side name omits the row id, so a name can be reused across
	// a soft-delete + recreate; a name that was an orphan in the snapshot may since
	// have been recreated under a now-active id — including a row created after the
	// snapshot, which activeIDs cannot see. Deleting it would destroy a live
	// namespace, so re-read the live label and confirm the id it carries is
	// genuinely gone or soft-deleted in the DB before removing it.
	for _, name := range plan.DeleteNames {
		current, err := h.shoot.GetNamespace(ctx, clusterID, name)
		if err != nil {
			errs = append(errs, fmt.Errorf("recheck orphan namespace %s: %w", name, err))
			continue
		}
		if current == nil {
			continue // already gone
		}
		rawID, ok := current.Labels[LabelNamespaceID]
		if !ok {
			continue // no longer fundament-managed; never touch
		}
		id, perr := uuid.Parse(rawID)
		if perr != nil {
			continue // malformed id label; leave it alone
		}
		row, gerr := h.queries.NamespaceGetForSync(ctx, db.NamespaceGetForSyncParams{ID: id})
		if gerr != nil && !errors.Is(gerr, pgx.ErrNoRows) {
			errs = append(errs, fmt.Errorf("recheck orphan namespace %s: %w", name, gerr))
			continue
		}
		if gerr == nil && !row.Deleted.Valid {
			continue // recreated and now active — not an orphan anymore
		}
		if err := h.shoot.DeleteNamespace(ctx, clusterID, name); err != nil {
			errs = append(errs, fmt.Errorf("delete orphan namespace %s: %w", name, err))
			continue
		}
		h.logger.Info("deleted orphan namespace", "cluster_id", clusterID, "name", name)
	}

	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("reconcile cluster %s: %w", clusterID, err)
	}
	return nil
}

// desiredLabels is the full managed label set for a namespace row. LabelNamespaceName
// records the fundament-side name as informational metadata so operators can correlate
// the resource back to fundament; the namespace name is immutable, so it never diverges
// from the generated resource name.
func desiredLabels(row *db.NamespaceGetForSyncRow) map[string]string {
	return map[string]string{
		LabelNamespaceID:    row.ID.String(),
		LabelNamespaceName:  row.Name,
		LabelProjectID:      row.ProjectID.String(),
		LabelOrganizationID: row.OrganizationID.String(),
		LabelClusterID:      row.ClusterID.String(),
		LabelManagedBy:      ManagedByValue,
	}
}

// Compile-time checks that Handler satisfies the registry interfaces.
var (
	_ handler.SyncHandler      = (*Handler)(nil)
	_ handler.ReconcileHandler = (*Handler)(nil)
)
