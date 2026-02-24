package cluster

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	"github.com/fundament-oss/fundament/common/dbconst"
)

// Config holds configuration for the cluster handler.
type Config struct {
	StatusBatchSize int32 `env:"STATUS_BATCH_SIZE" envDefault:"50"`
}

// Handler manages cluster lifecycle in Gardener (sync, status, orphan cleanup).
type Handler struct {
	queries  *db.Queries
	gardener gardener.Client
	logger   *slog.Logger
	cfg      Config
}

func New(pool db.DBTX, gardenerClient gardener.Client, logger *slog.Logger, cfg Config) *Handler {
	return &Handler{
		queries:  db.New(pool),
		gardener: gardenerClient,
		logger:   logger.With("handler", "cluster"),
		cfg:      cfg,
	}
}

// Sync processes a single cluster outbox row. Ported from worker-sync/worker.go:processOne.
// Errors are returned (outbox handles retry/backoff), not absorbed.
func (h *Handler) Sync(ctx context.Context, id uuid.UUID) error {
	// 1. Look up cluster
	row, err := h.queries.ClusterGetForSync(ctx, db.ClusterGetForSyncParams{ClusterID: id})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.logger.Info("cluster not found, skipping", "cluster_id", id)
			return nil
		}
		return fmt.Errorf("get cluster for sync: %w", err)
	}

	// 2. Determine action
	syncAction := dbconst.ClusterEventSyncAction_Sync
	var deleted *time.Time
	if row.Deleted.Valid {
		deleted = &row.Deleted.Time
		syncAction = dbconst.ClusterEventSyncAction_Delete
	}

	h.logger.Info("syncing cluster",
		"cluster_id", row.ID,
		"name", row.Name,
		"organization", row.OrganizationName,
		"action", syncAction)

	projectName := gardener.ProjectName(row.OrganizationName)

	// 3. Delete path: skip EnsureProject (D3), call DeleteShootByClusterID directly
	if syncAction == dbconst.ClusterEventSyncAction_Delete {
		if err := h.gardener.DeleteShootByClusterID(ctx, row.ID); err != nil {
			h.createSyncFailedEvent(ctx, row.ID, syncAction, err.Error())
			return fmt.Errorf("delete shoot: %w", err)
		}

		if err := h.queries.ClusterMarkSynced(ctx, db.ClusterMarkSyncedParams{ClusterID: row.ID}); err != nil {
			return fmt.Errorf("mark synced after delete: %w", err)
		}
		h.createSyncSucceededEvent(ctx, row.ID, syncAction)
		h.logger.Info("synced cluster deletion to gardener", "cluster_id", row.ID, "name", row.Name)
		return nil
	}

	// 4. Sync path: EnsureProject
	namespace, err := h.gardener.EnsureProject(ctx, projectName, row.OrganizationID)
	if err != nil {
		h.createSyncFailedEvent(ctx, row.ID, syncAction, "Failed to ensure Gardener project: "+err.Error())
		return fmt.Errorf("ensure project: %w", err)
	}

	// If namespace is empty, project was just created and Gardener hasn't set the namespace yet
	if namespace == "" {
		h.createSyncFailedEvent(ctx, row.ID, syncAction, "Waiting for organization namespace to be created")
		return fmt.Errorf("project namespace not ready yet for %s", projectName)
	}

	// 5. Build ClusterToSync with SyncAttempts: 0 (D4: field unused by gardener client)
	shootName := gardener.GenerateShootName(row.Name)
	clusterToSync := &gardener.ClusterToSync{
		ID:                row.ID,
		OrganizationID:    row.OrganizationID,
		OrganizationName:  row.OrganizationName,
		Name:              row.Name,
		ShootName:         shootName,
		Namespace:         namespace,
		Region:            row.Region,
		KubernetesVersion: row.KubernetesVersion,
		Deleted:           deleted,
		SyncAttempts:      0,
	}

	// 6. ApplyShoot
	if err := h.gardener.ApplyShoot(ctx, clusterToSync); err != nil {
		h.createSyncFailedEvent(ctx, row.ID, syncAction, err.Error())
		return fmt.Errorf("apply shoot: %w", err)
	}

	// 7. Success: mark synced + create event
	if err := h.queries.ClusterMarkSynced(ctx, db.ClusterMarkSyncedParams{ClusterID: row.ID}); err != nil {
		return fmt.Errorf("mark synced: %w", err)
	}
	h.createSyncSucceededEvent(ctx, row.ID, syncAction)

	h.logger.Info("synced cluster to gardener", "cluster_id", row.ID, "name", row.Name)
	return nil
}

// CheckStatus polls Gardener for shoot status updates. Ported from worker-status/worker.go.
func (h *Handler) CheckStatus(ctx context.Context) error {
	h.pollActiveClusters(ctx)
	h.pollDeletedClusters(ctx)
	return nil
}

// Reconcile performs periodic reconciliation. Ported from worker-sync/worker.go:reconcileAll.
func (h *Handler) Reconcile(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil
	}

	h.logger.Info("starting cluster reconciliation")

	dbClusters, err := h.queries.ClusterListActive(ctx)
	if err != nil {
		return fmt.Errorf("list active clusters: %w", err)
	}

	shoots, err := h.gardener.ListShoots(ctx)
	if err != nil {
		return fmt.Errorf("list shoots: %w", err)
	}

	dbClusterByID := make(map[uuid.UUID]db.ClusterListActiveRow)
	for _, c := range dbClusters {
		dbClusterByID[c.ID] = c
	}

	shootByClusterID := make(map[uuid.UUID]gardener.ShootInfo)
	for _, s := range shoots {
		if id, ok := s.Labels[gardener.LabelClusterID]; ok {
			clusterID, err := uuid.Parse(id)
			if err == nil {
				shootByClusterID[clusterID] = s
			}
		}
	}

	var driftedClusterCount int

	// Drift detection: re-enqueue via outbox instead of ClusterSyncReset
	for _, cluster := range dbClusters {
		if _, exists := shootByClusterID[cluster.ID]; !exists {
			h.logger.Warn("drift detected: shoot missing in Gardener",
				"cluster_id", cluster.ID, "name", cluster.Name)
			if err := h.queries.OutboxInsertReconcile(ctx, db.OutboxInsertReconcileParams{
				SubjectID:  cluster.ID,
				EntityType: string(handler.EntityCluster),
			}); err != nil {
				h.logger.Error("failed to insert reconcile outbox row", "error", err)
			}
			driftedClusterCount++
		}
	}

	// Orphaned Shoots (in Gardener but not in DB) are deleted
	for clusterID, shoot := range shootByClusterID {
		if _, exists := dbClusterByID[clusterID]; !exists {
			h.logger.Warn("deleting orphaned shoot in Gardener",
				"shoot", shoot.Name, "cluster_id", clusterID)
			if err := h.gardener.DeleteShootByClusterID(ctx, clusterID); err != nil {
				h.logger.Error("failed to delete orphaned shoot",
					"shoot", shoot.Name, "error", err)
			}
		}
	}

	h.logger.Info("cluster reconciliation complete",
		"clusters", len(dbClusters),
		"shoots", len(shoots),
		"drift_detected", driftedClusterCount)

	return nil
}

// pollActiveClusters checks active clusters for status changes.
// Ported from worker-status/worker.go:pollActiveClusters.
func (h *Handler) pollActiveClusters(ctx context.Context) {
	clusters, err := h.queries.ClusterListNeedingStatusCheck(ctx, db.ClusterListNeedingStatusCheckParams{
		LimitCount: h.cfg.StatusBatchSize,
	})
	if err != nil {
		h.logger.Error("failed to list clusters for status check", "error", err)
		return
	}

	for i := range clusters {
		cluster := &clusters[i]

		projectName := gardener.ProjectName(cluster.OrganizationName)
		namespace, err := h.gardener.EnsureProject(ctx, projectName, cluster.OrganizationID)
		if err != nil {
			h.logger.Error("failed to get project namespace",
				"cluster_id", cluster.ID,
				"error", err)
			continue
		}
		if namespace == "" {
			continue
		}

		clusterToSync := &gardener.ClusterToSync{
			ID:                cluster.ID,
			Name:              cluster.Name,
			OrganizationID:    cluster.OrganizationID,
			OrganizationName:  cluster.OrganizationName,
			Namespace:         namespace,
			Region:            cluster.Region,
			KubernetesVersion: cluster.KubernetesVersion,
		}

		shootStatus, err := h.gardener.GetShootStatus(ctx, clusterToSync)
		if err != nil {
			h.logger.Error("failed to get shoot status",
				"cluster_id", cluster.ID,
				"error", err)
			continue
		}

		var oldStatus gardener.ShootStatusType
		if cluster.ShootStatus.Valid {
			oldStatus = gardener.ShootStatusType(cluster.ShootStatus.String)
		}

		if err := h.queries.ClusterUpdateShootStatus(ctx, db.ClusterUpdateShootStatusParams{
			ClusterID: cluster.ID,
			Status:    pgtype.Text{String: string(shootStatus.Status), Valid: true},
			Message:   pgtype.Text{String: shootStatus.Message, Valid: true},
		}); err != nil {
			h.logger.Error("failed to update shoot status",
				"cluster_id", cluster.ID,
				"error", err)
			continue
		}

		if shootStatus.Status != oldStatus {
			var eventType dbconst.ClusterEventEventType
			switch shootStatus.Status {
			case gardener.StatusProgressing:
				eventType = dbconst.ClusterEventEventType_StatusProgressing
			case gardener.StatusReady:
				eventType = dbconst.ClusterEventEventType_StatusReady
			case gardener.StatusError:
				eventType = dbconst.ClusterEventEventType_StatusError
			case gardener.StatusPending, gardener.StatusDeleting:
				// No event for these transient states
			case gardener.StatusDeleted:
				// Handled in pollDeletedClusters
			default:
				h.logger.Warn("unhandled ShootStatusType", "status", shootStatus.Status, "cluster_id", cluster.ID)
			}

			if eventType != "" {
				if _, err := h.queries.ClusterCreateStatusEvent(ctx, db.ClusterCreateStatusEventParams{
					ClusterID: cluster.ID,
					EventType: string(eventType),
					Message:   pgtype.Text{String: shootStatus.Message, Valid: true},
				}); err != nil {
					h.logger.Warn("failed to create status event",
						"cluster_id", cluster.ID,
						"event_type", eventType,
						"error", err)
				}
			}
		}

		h.logger.Info("updated shoot status",
			"cluster_id", cluster.ID,
			"name", cluster.Name,
			"status", shootStatus.Status)

		if shootStatus.Status == gardener.StatusError {
			h.logger.Error("ALERT: shoot reconciliation failed",
				"cluster_id", cluster.ID,
				"name", cluster.Name,
				"message", shootStatus.Message)
		}
	}
}

// pollDeletedClusters checks deleted clusters to verify shoots are actually gone.
// Ported from worker-status/worker.go:pollDeletedClusters.
func (h *Handler) pollDeletedClusters(ctx context.Context) {
	clusters, err := h.queries.ClusterListDeletedNeedingVerification(ctx, db.ClusterListDeletedNeedingVerificationParams{
		LimitCount: h.cfg.StatusBatchSize,
	})
	if err != nil {
		h.logger.Error("failed to list deleted clusters for verification", "error", err)
		return
	}

	for i := range clusters {
		cluster := &clusters[i]
		var deleted *time.Time
		if cluster.Deleted.Valid {
			deleted = &cluster.Deleted.Time
		}

		projectName := gardener.ProjectName(cluster.OrganizationName)
		namespace, err := h.gardener.EnsureProject(ctx, projectName, cluster.OrganizationID)
		if err != nil {
			h.logger.Error("failed to get project namespace",
				"cluster_id", cluster.ID,
				"error", err)
			continue
		}
		if namespace == "" {
			continue
		}

		clusterToSync := &gardener.ClusterToSync{
			ID:                cluster.ID,
			Name:              cluster.Name,
			OrganizationID:    cluster.OrganizationID,
			OrganizationName:  cluster.OrganizationName,
			Namespace:         namespace,
			Region:            cluster.Region,
			KubernetesVersion: cluster.KubernetesVersion,
			Deleted:           deleted,
		}

		shootStatus, err := h.gardener.GetShootStatus(ctx, clusterToSync)
		if err != nil {
			h.logger.Error("failed to check deleted shoot status",
				"cluster_id", cluster.ID,
				"error", err)
			continue
		}

		if shootStatus.Status == gardener.StatusPending && shootStatus.Message == gardener.MsgShootNotFound {
			if err := h.queries.ClusterUpdateShootStatus(ctx, db.ClusterUpdateShootStatusParams{
				ClusterID: cluster.ID,
				Status:    pgtype.Text{String: string(gardener.StatusDeleted), Valid: true},
				Message:   pgtype.Text{String: "Shoot confirmed deleted", Valid: true},
			}); err != nil {
				h.logger.Error("failed to update deleted status",
					"cluster_id", cluster.ID,
					"error", err)
				continue
			}

			if _, err := h.queries.ClusterCreateStatusEvent(ctx, db.ClusterCreateStatusEventParams{
				ClusterID: cluster.ID,
				EventType: string(dbconst.ClusterEventEventType_StatusDeleted),
				Message:   pgtype.Text{String: "Shoot confirmed deleted", Valid: true},
			}); err != nil {
				h.logger.Warn("failed to create status_deleted event",
					"cluster_id", cluster.ID,
					"error", err)
			}

			h.logger.Info("confirmed shoot deletion",
				"cluster_id", cluster.ID,
				"name", cluster.Name)
		} else {
			if err := h.queries.ClusterUpdateShootStatus(ctx, db.ClusterUpdateShootStatusParams{
				ClusterID: cluster.ID,
				Status:    pgtype.Text{String: string(gardener.StatusDeleting), Valid: true},
				Message:   pgtype.Text{String: shootStatus.Message, Valid: true},
			}); err != nil {
				h.logger.Error("failed to update deleting status",
					"cluster_id", cluster.ID,
					"error", err)
			}
			h.logger.Debug("shoot still being deleted",
				"cluster_id", cluster.ID,
				"status", shootStatus.Status)
		}
	}
}

// createSyncFailedEvent records a sync_failed audit event (D2: every failed attempt gets one).
func (h *Handler) createSyncFailedEvent(ctx context.Context, clusterID uuid.UUID, syncAction dbconst.ClusterEventSyncAction, message string) {
	if _, err := h.queries.ClusterCreateSyncFailedEvent(ctx, db.ClusterCreateSyncFailedEventParams{
		ClusterID:  clusterID,
		SyncAction: pgtype.Text{String: string(syncAction), Valid: true},
		Message:    pgtype.Text{String: message, Valid: true},
		Attempt:    pgtype.Int4{}, // NULL — outbox tracks retries, not the handler
	}); err != nil {
		h.logger.Warn("failed to create sync_failed event", "cluster_id", clusterID, "error", err)
	}
}

// createSyncSucceededEvent records a sync_succeeded audit event.
func (h *Handler) createSyncSucceededEvent(ctx context.Context, clusterID uuid.UUID, syncAction dbconst.ClusterEventSyncAction) {
	if _, err := h.queries.ClusterCreateSyncSucceededEvent(ctx, db.ClusterCreateSyncSucceededEventParams{
		ClusterID:  clusterID,
		SyncAction: pgtype.Text{String: string(syncAction), Valid: true},
		Message:    pgtype.Text{},
	}); err != nil {
		h.logger.Warn("failed to create sync_succeeded event", "cluster_id", clusterID, "error", err)
	}
}
