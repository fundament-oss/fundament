package cluster

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/common/dbconst"
)

// Handler implements both SyncHandler and StatusHandler for cluster (Shoot) lifecycle.
// The cluster worker role (fun_cluster_worker) has unrestricted SELECT on organizations,
// projects, and project_members because it needs to resolve organization names and
// project membership across all tenants when syncing clusters to Gardener.
type Handler struct {
	queries  *db.Queries
	gardener gardener.Client
	logger   *slog.Logger
}

func New(queries *db.Queries, gardenerClient gardener.Client, logger *slog.Logger) *Handler {
	return &Handler{
		queries:  queries,
		gardener: gardenerClient,
		logger:   logger.With("handler", "cluster"),
	}
}

// Sync processes an outbox row for a cluster. It ensures the Gardener Project exists,
// then creates/updates or deletes the Shoot in Gardener.
func (h *Handler) Sync(ctx context.Context, clusterID uuid.UUID) error {
	cluster, err := h.queries.ClusterGetForSync(ctx, db.ClusterGetForSyncParams{ClusterID: clusterID})
	if err != nil {
		return fmt.Errorf("get cluster for sync: %w", err)
	}

	isDelete := cluster.Deleted.Valid
	syncAction := dbconst.ClusterEventSyncAction_Sync
	if isDelete {
		syncAction = dbconst.ClusterEventSyncAction_Delete
	}

	h.logger.Info("syncing cluster",
		"cluster_id", clusterID,
		"name", cluster.Name,
		"organization", cluster.OrganizationName,
		"action", syncAction)

	projectName := gardener.ProjectName(cluster.OrganizationName)

	namespace, err := h.gardener.EnsureProject(ctx, projectName, cluster.OrganizationID)
	if err != nil {
		h.markSyncFailed(ctx, clusterID, "ensure project: "+err.Error(), syncAction)
		return fmt.Errorf("ensure project: %w", err)
	}

	if namespace == "" {
		h.markSyncFailed(ctx, clusterID, "project namespace not ready yet", syncAction)
		return fmt.Errorf("project namespace not ready yet")
	}

	shootName := gardener.GenerateShootName(cluster.Name)

	clusterToSync := gardener.ClusterToSync{
		ID:                clusterID,
		OrganizationID:    cluster.OrganizationID,
		OrganizationName:  cluster.OrganizationName,
		Name:              cluster.Name,
		ShootName:         shootName,
		Namespace:         namespace,
		Region:            cluster.Region,
		KubernetesVersion: cluster.KubernetesVersion,
	}
	if cluster.Deleted.Valid {
		t := cluster.Deleted.Time
		clusterToSync.Deleted = &t
	}

	var syncErr error
	if isDelete {
		syncErr = h.gardener.DeleteShootByClusterID(ctx, clusterID)
	} else {
		syncErr = h.gardener.ApplyShoot(ctx, &clusterToSync)
	}

	if syncErr != nil {
		h.markSyncFailed(ctx, clusterID, syncErr.Error(), syncAction)
		return fmt.Errorf("gardener sync: %w", syncErr)
	}

	if err := h.queries.ClusterMarkSynced(ctx, db.ClusterMarkSyncedParams{
		ClusterID: clusterID,
	}); err != nil {
		return fmt.Errorf("mark synced: %w", err)
	}

	if _, err := h.queries.ClusterCreateSyncSucceededEvent(ctx, db.ClusterCreateSyncSucceededEventParams{
		ClusterID:  clusterID,
		SyncAction: pgtype.Text{String: string(syncAction), Valid: true},
		Message:    pgtype.Text{},
	}); err != nil {
		h.logger.Warn("failed to create sync_succeeded event", "error", err)
	}

	h.logger.Info("synced cluster to gardener",
		"cluster_id", clusterID,
		"name", cluster.Name,
		"action", syncAction)

	return nil
}

// markSyncFailed records a sync_failed event for audit history.
// Retry tracking is handled by the outbox table, not the clusters table.
func (h *Handler) markSyncFailed(ctx context.Context, clusterID uuid.UUID, errMsg string, syncAction dbconst.ClusterEventSyncAction) {
	if _, err := h.queries.ClusterCreateSyncFailedEvent(ctx, db.ClusterCreateSyncFailedEventParams{
		ClusterID:  clusterID,
		SyncAction: pgtype.Text{String: string(syncAction), Valid: true},
		Message:    pgtype.Text{String: errMsg, Valid: true},
	}); err != nil {
		h.logger.Warn("failed to create sync_failed event", "error", err)
	}
}
