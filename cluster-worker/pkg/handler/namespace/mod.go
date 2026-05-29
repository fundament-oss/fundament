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

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
)

// Label keys applied to every fundament-managed namespace on a shoot. The
// fundament.io/ prefix matches the existing convention (usersync uses
// fundament.io/user-id). LabelNamespaceID is the canonical ownership marker:
// reconcile and delete only ever touch namespaces carrying it.
const (
	LabelNamespaceID    = "fundament.io/namespace-id"
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
func (h *Handler) ensure(ctx context.Context, row *db.NamespaceGetForSyncRow) error {
	desired := desiredLabels(row)

	existing, err := h.shoot.GetNamespace(ctx, row.ClusterID, row.Name)
	if err != nil {
		return fmt.Errorf("get namespace %s: %w", row.Name, err)
	}

	if existing == nil {
		// The target name is absent. If this row previously had a different name
		// (an unsupported rename), remove the stale cluster-side namespace first.
		if err := h.deleteRenamed(ctx, row); err != nil {
			return err
		}
		if err := h.shoot.CreateNamespace(ctx, row.ClusterID, row.Name, desired); err != nil {
			return fmt.Errorf("create namespace %s: %w", row.Name, err)
		}
		h.logger.Info("created namespace",
			"namespace_id", row.ID, "cluster_id", row.ClusterID, "name", row.Name)
		return nil
	}

	// The name exists. Only adopt it if it already carries our id; otherwise it
	// is someone else's namespace and adopting it would be dangerous.
	if existing.Labels[LabelNamespaceID] != row.ID.String() {
		return fmt.Errorf("namespace name collision: %s already exists on shoot without matching label", row.Name)
	}

	if err := h.shoot.UpdateNamespaceLabels(ctx, row.ClusterID, row.Name, desired); err != nil {
		return fmt.Errorf("update namespace %s labels: %w", row.Name, err)
	}
	h.logger.Debug("namespace present, labels reconciled",
		"namespace_id", row.ID, "cluster_id", row.ClusterID, "name", row.Name)
	return nil
}

// delete hard-deletes the cluster-side namespace for a soft-deleted row, but
// only when it carries our matching label. A name match without a label match
// means the namespace is not ours — log and skip rather than destroy it.
func (h *Handler) delete(ctx context.Context, row *db.NamespaceGetForSyncRow) error {
	existing, err := h.shoot.GetNamespace(ctx, row.ClusterID, row.Name)
	if err != nil {
		return fmt.Errorf("get namespace %s: %w", row.Name, err)
	}
	if existing == nil {
		return nil // already gone — idempotent
	}
	if existing.Labels[LabelNamespaceID] != row.ID.String() {
		h.logger.Warn("refusing to delete namespace without matching label",
			"namespace_id", row.ID, "cluster_id", row.ClusterID, "name", row.Name,
			"found_label", existing.Labels[LabelNamespaceID])
		return nil
	}
	if err := h.shoot.DeleteNamespace(ctx, row.ClusterID, row.Name); err != nil {
		return fmt.Errorf("delete namespace %s: %w", row.Name, err)
	}
	h.logger.Info("deleted namespace",
		"namespace_id", row.ID, "cluster_id", row.ClusterID, "name", row.Name)
	return nil
}

// deleteRenamed removes any cluster-side namespace that carries this row's id
// label but a different name — the residue of a (rare, unsupported) rename.
func (h *Handler) deleteRenamed(ctx context.Context, row *db.NamespaceGetForSyncRow) error {
	existing, err := h.shoot.ListNamespaces(ctx, row.ClusterID, LabelNamespaceID)
	if err != nil {
		return fmt.Errorf("list namespaces: %w", err)
	}
	for i := range existing {
		if existing[i].Labels[LabelNamespaceID] == row.ID.String() && existing[i].Name != row.Name {
			if err := h.shoot.DeleteNamespace(ctx, row.ClusterID, existing[i].Name); err != nil {
				return fmt.Errorf("delete renamed namespace %s: %w", existing[i].Name, err)
			}
			h.logger.Info("deleted renamed namespace",
				"namespace_id", row.ID, "old_name", existing[i].Name, "new_name", row.Name)
		}
	}
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
	for _, name := range plan.DeleteNames {
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

// desiredLabels is the full managed label set for a namespace row.
func desiredLabels(row *db.NamespaceGetForSyncRow) map[string]string {
	return map[string]string{
		LabelNamespaceID:    row.ID.String(),
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
