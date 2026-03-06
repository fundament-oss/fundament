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
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	"github.com/fundament-oss/fundament/common/dbconst"
)

// Config holds handler-specific configuration.
type Config struct {
	StatusBatchSize int32 `env:"STATUS_BATCH_SIZE" envDefault:"50"`
	MaxRetries      int32 `env:"MAX_RETRIES" envDefault:"10"`
}

// Handler manages cluster lifecycle in Gardener (sync, status, orphan cleanup).
type Handler struct {
	pool     *pgxpool.Pool
	queries  *db.Queries
	gardener gardener.Client
	logger   *slog.Logger
	cfg      Config
}

func New(pool *pgxpool.Pool, gardenerClient gardener.Client, logger *slog.Logger, cfg Config) *Handler {
	return &Handler{
		pool:     pool,
		queries:  db.New(pool),
		gardener: gardenerClient,
		logger:   logger.With("handler", "cluster"),
		cfg:      cfg,
	}
}

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

// CheckStatus polls Gardener for shoot status and updates the database.
func (h *Handler) CheckStatus(ctx context.Context) error {
	var errs []error
	if err := h.pollActiveClusters(ctx); err != nil {
		errs = append(errs, err)
	}
	if err := h.pollDeletedClusters(ctx); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// Reconcile compares DB state with Gardener state to detect drift and clean up orphans.
func (h *Handler) Reconcile(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil
	}

	h.logger.Info("starting cluster reconciliation")

	dbClusters, err := h.queries.ClusterListActive(ctx)
	if err != nil {
		h.logger.Error("failed to list clusters from DB", "error", err)
		return fmt.Errorf("list active clusters: %w", err)
	}

	shoots, err := h.gardener.ListShoots(ctx)
	if err != nil {
		h.logger.Error("failed to list shoots from Gardener", "error", err)
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

	// Drift detection: synced clusters missing in Gardener
	var driftedCount int
	for _, cluster := range dbClusters {
		if cluster.HasCompletedOutbox {
			if _, exists := shootByClusterID[cluster.ID]; !exists {
				h.logger.Warn("drift detected: shoot missing in Gardener",
					"cluster_id", cluster.ID, "name", cluster.Name)
				if err := h.queries.OutboxInsertReconcile(ctx, db.OutboxInsertReconcileParams{
					ClusterID:  pgtype.UUID{Bytes: cluster.ID, Valid: true},
					MaxRetries: h.cfg.MaxRetries,
				}); err != nil {
					h.logger.Error("failed to insert reconcile outbox row", "cluster_id", cluster.ID, "error", err)
				}
				driftedCount++
			}
		}
	}

	// Orphan cleanup: shoots in Gardener without a DB cluster
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
		"drift_detected", driftedCount)

	return nil
}

// pollActiveClusters checks Gardener status for active (non-deleted) clusters.
func (h *Handler) pollActiveClusters(ctx context.Context) error {
	clusters, err := h.queries.ClusterListNeedingStatusCheck(ctx, db.ClusterListNeedingStatusCheckParams{
		LimitCount: h.cfg.StatusBatchSize,
	})
	if err != nil {
		h.logger.Error("failed to list clusters for status check", "error", err)
		return fmt.Errorf("list clusters for status check: %w", err)
	}

	for i := range clusters {
		if ctx.Err() != nil {
			return nil
		}
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
				panic(fmt.Sprintf("unhandled shoot status: %s", shootStatus.Status))
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
	return nil
}

// pollDeletedClusters verifies that soft-deleted clusters have actually been removed from Gardener.
func (h *Handler) pollDeletedClusters(ctx context.Context) error {
	clusters, err := h.queries.ClusterListDeletedNeedingVerification(ctx, db.ClusterListDeletedNeedingVerificationParams{
		LimitCount: h.cfg.StatusBatchSize,
	})
	if err != nil {
		h.logger.Error("failed to list deleted clusters for verification", "error", err)
		return fmt.Errorf("list deleted clusters for verification: %w", err)
	}

	for i := range clusters {
		if ctx.Err() != nil {
			return nil
		}
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

// createSyncFailedEvent creates a sync_failed event (D2: every attempt gets an audit event).
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
