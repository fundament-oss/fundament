package cluster

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	"github.com/fundament-oss/fundament/common/dbconst"
)

// Sync processes a single cluster outbox row by syncing the cluster to Gardener.
// Returns nil to mark the row as processed, or an error to trigger outbox retry.
func (h *Handler) Sync(ctx context.Context, id uuid.UUID, sc handler.SyncContext) error {
	// 1. Look up cluster
	cluster, err := h.queries.ClusterGetForSync(ctx, db.ClusterGetForSyncParams{ClusterID: id})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.logger.Info("cluster not found, skipping (deleted between outbox insert and processing)", "cluster_id", id)
			return nil
		}
		return fmt.Errorf("get cluster for sync: %w", err)
	}

	// 2. Determine action
	var deleted *time.Time
	if cluster.Deleted.Valid {
		deleted = &cluster.Deleted.Time
	}

	syncAction := dbconst.ClusterEventSyncAction_Sync
	if deleted != nil {
		syncAction = dbconst.ClusterEventSyncAction_Delete
	}

	h.logger.Info("syncing cluster",
		"cluster_id", cluster.ID,
		"name", cluster.Name,
		"organization", cluster.OrganizationName,
		"deleted", deleted != nil,
		"action", syncAction)

	// 3. Delete path (D3): skip EnsureProject, search by label across all namespaces
	if syncAction == dbconst.ClusterEventSyncAction_Delete {
		if err := h.gardener.DeleteShootByClusterID(ctx, cluster.ID); err != nil {
			h.createSyncFailedEvent(ctx, cluster.ID, syncAction, err.Error())
			return fmt.Errorf("delete shoot: %w", err)
		}

		h.createSyncSucceededEvent(ctx, cluster.ID, syncAction, syncMessage(sc.Event, sc.Source))
		h.logger.Info("synced cluster deletion to gardener", "cluster_id", cluster.ID, "name", cluster.Name)
		return nil
	}

	// 4. Sync path: ensure project exists
	projectName := gardener.ProjectName(cluster.OrganizationName)
	namespace, err := h.gardener.EnsureProject(ctx, projectName, cluster.OrganizationID)
	if err != nil {
		h.createSyncFailedEvent(ctx, cluster.ID, syncAction, "ensure project: "+err.Error())
		return fmt.Errorf("ensure project: %w", err)
	}
	if namespace == "" {
		h.createSyncFailedEvent(ctx, cluster.ID, syncAction, "project namespace not ready yet")
		return fmt.Errorf("project namespace not ready for %s", projectName)
	}

	// 5. Load node pools
	nodePoolRows, err := h.queries.NodePoolListByClusterID(ctx, db.NodePoolListByClusterIDParams{ClusterID: cluster.ID})
	if err != nil {
		h.createSyncFailedEvent(ctx, cluster.ID, syncAction, "load node pools: "+err.Error())
		return fmt.Errorf("load node pools: %w", err)
	}

	// 6. Build ClusterToSync and apply (D4: SyncAttempts = 0)
	shootName := gardener.GenerateShootName(cluster.Name, cluster.ID)
	clusterToSync := &gardener.ClusterToSync{
		ID:                cluster.ID,
		OrganizationID:    cluster.OrganizationID,
		OrganizationName:  cluster.OrganizationName,
		Name:              cluster.Name,
		ShootName:         shootName,
		Namespace:         namespace,
		Region:            cluster.Region,
		KubernetesVersion: cluster.KubernetesVersion,
		Deleted:           deleted,
		SyncAttempts:      0,
		NodePools:         toGardenerNodePools(nodePoolRows),
	}

	if err := h.gardener.ApplyShoot(ctx, clusterToSync); err != nil {
		h.createSyncFailedEvent(ctx, cluster.ID, syncAction, err.Error())
		return fmt.Errorf("apply shoot: %w", err)
	}

	// 7. Success
	h.createSyncSucceededEvent(ctx, cluster.ID, syncAction, syncMessage(sc.Event, sc.Source))
	h.logger.Info("synced cluster to gardener", "cluster_id", cluster.ID, "name", cluster.Name)
	return nil
}

// toGardenerNodePools converts DB rows to the gardener.NodePool slice expected by ClusterToSync.
func toGardenerNodePools(rows []db.NodePoolListByClusterIDRow) []gardener.NodePool {
	pools := make([]gardener.NodePool, len(rows))
	for i, np := range rows {
		pools[i] = gardener.NodePool{
			Name:         np.Name,
			MachineType:  np.MachineType,
			AutoscaleMin: np.AutoscaleMin,
			AutoscaleMax: np.AutoscaleMax,
		}
	}
	return pools
}

// createSyncFailedEvent creates a sync_failed audit event.
// The error is still returned to the outbox worker for retry handling.
func (h *Handler) createSyncFailedEvent(ctx context.Context, clusterID uuid.UUID, action dbconst.ClusterEventSyncAction, msg string) {
	if _, err := h.queries.ClusterCreateSyncFailedEvent(ctx, db.ClusterCreateSyncFailedEventParams{
		ClusterID:  clusterID,
		SyncAction: pgtype.Text{String: string(action), Valid: true},
		Message:    pgtype.Text{String: msg, Valid: true},
		Attempt:    pgtype.Int4{}, // NULL — outbox tracks retries, not the handler
	}); err != nil {
		h.logger.Warn("failed to create sync_failed event", "cluster_id", clusterID, "error", err)
	}
}

// createSyncSucceededEvent creates a sync_succeeded event.
func (h *Handler) createSyncSucceededEvent(ctx context.Context, clusterID uuid.UUID, action dbconst.ClusterEventSyncAction, message string) {
	if _, err := h.queries.ClusterCreateSyncSucceededEvent(ctx, db.ClusterCreateSyncSucceededEventParams{
		ClusterID:  clusterID,
		SyncAction: pgtype.Text{String: string(action), Valid: true},
		Message:    pgtype.Text{String: message, Valid: message != ""},
	}); err != nil {
		h.logger.Warn("failed to create sync_succeeded event", "cluster_id", clusterID, "error", err)
	}
}

// syncMessage builds a human-readable message from the outbox event and source.
func syncMessage(event, source string) string {
	entity := "Cluster"
	if source == "node_pool" {
		entity = "Node pool"
	}
	switch event {
	case "created":
		return entity + " created"
	case "updated":
		return entity + " updated"
	case "deleted":
		return entity + " deleted"
	case "reconcile":
		return entity + " reconciled"
	default:
		panic(fmt.Sprintf("unhandled sync event: %q (source: %q)", event, source))
	}
}
