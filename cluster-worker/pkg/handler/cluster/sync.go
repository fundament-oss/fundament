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
	"github.com/fundament-oss/fundament/common/kubename"
)

// Sync dispatches an outbox row to the appropriate sync method based on entity type.
// Preconditions are checked before any sync logic executes.
func (h *Handler) Sync(ctx context.Context, id uuid.UUID, sc handler.SyncContext) error {
	if err := h.checkPreconditions(ctx, sc.EntityType, id); err != nil {
		return err
	}

	switch sc.EntityType {
	case handler.EntityCluster:
		return h.syncCluster(ctx, id, sc)
	case handler.EntityNodePool:
		return h.syncNodePool(ctx, id, sc)
	default:
		panic(fmt.Sprintf("unhandled entity type %q in cluster handler", sc.EntityType))
	}
}

// MsgWaitingForNodePool is the reason a new cluster's row parks, and the
// message of the sync_waiting_for_precondition event it records.
const MsgWaitingForNodePool = "Creating cluster: waiting for a node pool"

// waitingForFirstNodePool reports whether a cluster has to wait for a node
// pool before it reaches Gardener: it is live, has never been synced, and has
// no live pool. A Shoot is never created without a worker.
// The pools are read first: a cluster with one, the common case, costs one query.
// q is the handler's queries, or the park transaction's for the recheck.
func (h *Handler) waitingForFirstNodePool(ctx context.Context, q *db.Queries, clusterID uuid.UUID) (bool, error) {
	pools, err := q.NodePoolListByClusterID(ctx, db.NodePoolListByClusterIDParams{ClusterID: clusterID})
	if err != nil {
		return false, fmt.Errorf("load node pools: %w", err)
	}
	if len(pools) > 0 {
		return false, nil
	}

	cluster, err := q.ClusterGetForSync(ctx, db.ClusterGetForSyncParams{ClusterID: clusterID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil // cluster not found — syncCluster will handle gracefully
		}
		return false, fmt.Errorf("get cluster for node pool check: %w", err)
	}
	if cluster.Deleted.Valid {
		return false, nil
	}

	synced, err := q.ClusterHasEverBeenSynced(ctx, db.ClusterHasEverBeenSyncedParams{
		ClusterID: pgtype.UUID{Bytes: clusterID, Valid: true},
	})
	if err != nil {
		return false, fmt.Errorf("check cluster ever synced: %w", err)
	}
	return !synced, nil
}

// syncNodePool resolves a node pool ID to its parent cluster and syncs the cluster.
// Returns nil if the node pool no longer exists (deleted between outbox insert and processing).
//
// A cluster that never reached Gardener is created by its own outbox row, which
// the node pool trigger woke when the pool was inserted, and which reads every
// pool when it runs. Its pools' rows only make sure such a row is queued (an
// earlier one may have failed for good), so no Shoot is created twice.
func (h *Handler) syncNodePool(ctx context.Context, nodePoolID uuid.UUID, sc handler.SyncContext) error {
	clusterID, err := h.queries.NodePoolGetClusterID(ctx, db.NodePoolGetClusterIDParams{NodePoolID: nodePoolID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.logger.Info("node pool not found, skipping (deleted between outbox insert and processing)", "node_pool_id", nodePoolID)
			return nil
		}
		return fmt.Errorf("get cluster_id for node pool %s: %w", nodePoolID, err)
	}

	synced, err := h.queries.ClusterHasEverBeenSynced(ctx, db.ClusterHasEverBeenSyncedParams{
		ClusterID: pgtype.UUID{Bytes: clusterID, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("check cluster ever synced: %w", err)
	}
	if !synced {
		queued, err := h.ensureClusterSyncQueued(ctx, clusterID)
		if err != nil {
			return fmt.Errorf("queue cluster sync for node pool %s: %w", nodePoolID, err)
		}
		h.logger.Info("cluster not in Gardener yet, leaving the node pool to the cluster's own sync",
			"node_pool_id", nodePoolID, "cluster_id", clusterID, "queued_new_sync", queued)
		return nil
	}

	return h.syncCluster(ctx, clusterID, sc)
}

// ensureClusterSyncQueued makes sure a sync of the cluster is still to come,
// one that will read the pool being handled: a runnable row no worker holds
// will, so nothing is queued then. A row another worker is processing may
// have read the pools before this one was inserted, so it does not count, and
// a new sync is queued (ApplyShoot creates or updates, so a second one is
// harmless). It reports whether a row was queued.
func (h *Handler) ensureClusterSyncQueued(ctx context.Context, clusterID uuid.UUID) (bool, error) {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	qtx := h.queries.WithTx(tx)

	unclaimed, err := qtx.OutboxListUnclaimedClusterSyncs(ctx, db.OutboxListUnclaimedClusterSyncsParams{
		ClusterID: pgtype.UUID{Bytes: clusterID, Valid: true},
	})
	if err != nil {
		return false, fmt.Errorf("list unclaimed cluster syncs: %w", err)
	}
	if len(unclaimed) > 0 {
		return false, nil
	}
	if err := qtx.OutboxInsertClusterSync(ctx, db.OutboxInsertClusterSyncParams{
		ClusterID: pgtype.UUID{Bytes: clusterID, Valid: true},
	}); err != nil {
		return false, fmt.Errorf("insert cluster sync: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}
	return true, nil
}

// syncCluster processes a single cluster outbox row by syncing the cluster to Gardener.
// Returns nil to mark the row as processed, or an error to trigger outbox retry.
func (h *Handler) syncCluster(ctx context.Context, id uuid.UUID, sc handler.SyncContext) error {
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
		found, err := h.gardener.DeleteShootByClusterID(ctx, cluster.ID)
		if err != nil {
			return h.syncError(ctx, cluster.ID, syncAction, "delete shoot", err)
		}
		h.createSyncSucceededEvent(ctx, cluster.ID, syncAction, syncMessage(sc.Event, sc.EntityType))
		if !found {
			// Nothing to wait for: a cluster deleted before it reached Gardener
			// (while it waited for its first pool, say) would otherwise count
			// against the organization's cluster quota until the next status
			// sweep confirms there is no Shoot.
			if err := h.markShootDeleted(ctx, cluster.ID); err != nil {
				return h.syncError(ctx, cluster.ID, syncAction, "mark deleted", err)
			}
		}
		h.logger.Info("synced cluster deletion to gardener", "cluster_id", cluster.ID, "name", cluster.Name, "shoot_found", found)
		return nil
	}

	// 4. Sync path: resolve project namespace (precondition guarantees it's ready)
	namespace, err := h.resolveProjectNamespace(ctx, cluster.OrganizationName, cluster.OrganizationID)
	if err != nil {
		return h.syncError(ctx, cluster.ID, syncAction, "ensure project", err)
	}

	// 5. Load node pools
	nodePoolRows, err := h.queries.NodePoolListByClusterID(ctx, db.NodePoolListByClusterIDParams{ClusterID: cluster.ID})
	if err != nil {
		return h.syncError(ctx, cluster.ID, syncAction, "load node pools", err)
	}
	// A cluster not in Gardener yet waits for its first pool (a precondition), and
	// the API never removes a cluster's last pool, so this is a cluster that kept
	// a Shoot after losing every pool some other way. Its Shoot keeps the workers
	// it has.
	if len(nodePoolRows) == 0 {
		// Fails at once: a retry cannot add the pool, and a cluster from before
		// pools were required gets one from its owner, not from time.
		return h.syncError(ctx, cluster.ID, syncAction, "load node pools", handler.NonRetryable(gardener.ErrNoNodePools))
	}

	// 6. Build ClusterToSync and apply
	clusterToSync := clusterToSyncBase(cluster.ID, cluster.Name, cluster.OrganizationName, cluster.OrganizationID, namespace, cluster.Region, cluster.KubernetesVersion, cluster.CloudProfile, cluster.CloudProfileRegion)
	clusterToSync.ShootName = kubename.GenerateShootName(cluster.Name, cluster.ID)
	clusterToSync.Deleted = deleted
	clusterToSync.NodePools = toGardenerNodePools(nodePoolRows)

	// A Shoot that went missing since the last status check is recreated
	// below: check first, so it is recorded as lost. Otherwise the new, empty
	// Shoot would read as an update of the cluster that was there.
	if _, err := h.checkCluster(ctx, cluster.ID); err != nil && !errors.Is(err, ErrClusterNotSynced) {
		return h.syncError(ctx, cluster.ID, syncAction, "check shoot status", err)
	}

	specChanged, err := h.gardener.ApplyShoot(ctx, clusterToSync)
	if err != nil {
		return h.syncError(ctx, cluster.ID, syncAction, "apply shoot", err)
	}

	// 7. Success
	h.createSyncSucceededEvent(ctx, cluster.ID, syncAction, syncMessage(sc.Event, sc.EntityType))
	if specChanged {
		h.markShootUpdating(ctx, cluster.ID)
	}
	h.logger.Info("synced cluster to gardener", "cluster_id", cluster.ID, "name", cluster.Name)
	return nil
}

// msgNoShootToDelete is the status message of a deleted cluster that had no
// Shoot to delete.
const msgNoShootToDelete = "Deleted before it reached Gardener; no Shoot to remove"

// markShootDeleted records, in one transaction, that a deleted cluster's Shoot
// is gone: the status and its status_deleted event, as the status sweep does
// when it confirms a deletion.
func (h *Handler) markShootDeleted(ctx context.Context, clusterID uuid.UUID) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	qtx := h.queries.WithTx(tx)

	marked, err := qtx.ClusterMarkShootDeleted(ctx, db.ClusterMarkShootDeletedParams{
		ClusterID: clusterID,
		Message:   pgtype.Text{String: msgNoShootToDelete, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("mark shoot deleted: %w", err)
	}
	if marked == 0 {
		return nil
	}
	if _, err := qtx.ClusterCreateStatusEvent(ctx, db.ClusterCreateStatusEventParams{
		ClusterID: clusterID,
		EventType: string(dbconst.ClusterEventEventType_StatusDeleted),
		Message:   pgtype.Text{String: msgNoShootToDelete, Valid: true},
	}); err != nil {
		return fmt.Errorf("create status_deleted event: %w", err)
	}
	return tx.Commit(ctx)
}

// markShootUpdating marks a ready cluster as updating once Gardener accepted
// a spec change, so the reconcile it starts is visible; the cluster stays
// ready and usable meanwhile. A routine reconcile Gardener starts by itself
// is not an update. The sync has succeeded either way, so failures are only
// logged.
func (h *Handler) markShootUpdating(ctx context.Context, clusterID uuid.UUID) {
	marked, err := h.queries.ClusterMarkShootUpdating(ctx, db.ClusterMarkShootUpdatingParams{
		ClusterID:       clusterID,
		Message:         pgtype.Text{String: gardener.MsgShootUpdatePending, Valid: true},
		AwaitingMessage: pgtype.Text{String: gardener.MsgShootAwaitingHealth, Valid: true},
	})
	if err != nil {
		h.logger.Warn("failed to mark cluster as updating", "cluster_id", clusterID, "error", err)
		return
	}
	if marked == 0 {
		return
	}
	if _, err := h.queries.ClusterCreateStatusEvent(ctx, db.ClusterCreateStatusEventParams{
		ClusterID: clusterID,
		EventType: string(dbconst.ClusterEventEventType_StatusProgressing),
		Message:   pgtype.Text{String: gardener.MsgShootUpdatePending, Valid: true},
	}); err != nil {
		h.logger.Warn("failed to create status_progressing event", "cluster_id", clusterID, "error", err)
	}
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

// syncError logs an error, creates a sync_failed audit event, and returns a wrapped error.
// This centralizes the verbose "log + event + return" pattern used throughout the sync path.
func (h *Handler) syncError(ctx context.Context, clusterID uuid.UUID, syncAction dbconst.ClusterEventSyncAction, operation string, err error) error {
	h.logger.Error("sync failed: "+operation, "cluster_id", clusterID, "error", err)
	h.createSyncFailedEvent(ctx, clusterID, syncAction, operation+": "+err.Error())
	return fmt.Errorf("%s: %w", operation, err)
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

// syncMessage builds a human-readable message from the outbox event and entity type.
func syncMessage(event dbconst.ClusterOutboxEvent, entityType handler.EntityType) string {
	entity := "Cluster"
	if entityType == handler.EntityNodePool {
		entity = "Node pool"
	}
	switch event {
	case dbconst.ClusterOutboxEvent_Created:
		return entity + " created"
	case dbconst.ClusterOutboxEvent_Updated:
		return entity + " updated"
	case dbconst.ClusterOutboxEvent_Deleted:
		return entity + " deleted"
	case dbconst.ClusterOutboxEvent_Reconcile:
		return entity + " reconciled"
	default:
		panic(fmt.Sprintf("unhandled sync event: %q (entity: %q)", event, entityType))
	}
}
