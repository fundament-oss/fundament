// Package worker implements the cluster sync worker.
// It listens for PostgreSQL notifications and syncs cluster state to Gardener.
package worker_sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/gardener"
)

// SyncWorker syncs cluster state from PostgreSQL to Gardener.
type SyncWorker struct {
	pool     *pgxpool.Pool
	queries  *db.Queries
	gardener gardener.Client
	logger   *slog.Logger
	cfg      Config
	workerID string // Unique identifier for this worker instance (for debugging)

	ready atomic.Bool // For health checks
}

type Config struct {
	PollInterval      time.Duration `env:"POLL_INTERVAL" envDefault:"30s"`     // Timeout for WaitForNotification
	ReconcileInterval time.Duration `env:"RECONCILE_INTERVAL" envDefault:"5m"` // How often to run full reconciliation
	MaxAttempts       int32         `env:"MAX_ATTEMPTS" envDefault:"5"`        // Max retries before giving up
	BackoffDelay      time.Duration `env:"BACKOFF_DELAY" envDefault:"5s"`      // Delay on reconnect and error backoff
}

type triggerType int

const (
	triggerNotification triggerType = iota
	triggerTimeout
)

func New(pool *pgxpool.Pool, gardenerClient gardener.Client, logger *slog.Logger, cfg Config) *SyncWorker {
	// Generate a unique worker ID for debugging (hostname-pid)
	hostname, _ := os.Hostname()
	workerID := fmt.Sprintf("%s-%d", hostname, os.Getpid())

	return &SyncWorker{
		pool:     pool,
		queries:  db.New(pool),
		gardener: gardenerClient,
		logger:   logger.With("worker_id", workerID),
		cfg:      cfg,
		workerID: workerID,
	}
}

// Run starts the worker with automatic reconnection on LISTEN connection loss.
func (w *SyncWorker) Run(ctx context.Context) error {
	for {
		err := w.runWithConnection(ctx)
		if ctx.Err() != nil {
			return fmt.Errorf("worker stopped: %w", ctx.Err())
		}
		w.logger.Error("connection lost, reconnecting", "error", err, "delay", w.cfg.BackoffDelay)
		w.ready.Store(false)
		time.Sleep(w.cfg.BackoffDelay)
	}
}

// runWithConnection handles a single LISTEN connection lifecycle.
//
// # Event Loop Design
//
// The worker uses PostgreSQL LISTEN/NOTIFY for event-driven processing with
// periodic polling as a fallback. To prevent missed notifications and detect drift between DB and Gardener
//
// # Timing
//
// The loop wakes up on whichever comes first:
//   - A pg_notify('cluster_sync', cluster_id) notification
//   - PollInterval timeout (default 30s) - fallback if notifications missed
//
// Reconciliation runs every ReconcileInterval (default 5m), checked after each
// loop iteration. Actual interval is ReconcileInterval ± PollInterval.
func (w *SyncWorker) runWithConnection(ctx context.Context) error {
	// --- Setup: establish LISTEN connection ---
	conn, err := w.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire listen connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN cluster_sync"); err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	w.logger.Info("listening for cluster_sync notifications")
	w.ready.Store(true)

	// --- Startup: process any work missed while offline ---
	w.processAllPending(ctx)
	lastReconcile := time.Now()

	for {
		_, err := w.waitForTrigger(ctx, conn)
		if err != nil {
			return err
		}

		w.processAllPending(ctx)

		if time.Since(lastReconcile) >= w.cfg.ReconcileInterval {
			w.reconcileAll(ctx)
			lastReconcile = time.Now()
		}
	}
}

// waitForTrigger blocks until a notification arrives or PollInterval elapses.
// Returns the trigger type, or an error if the context is canceled or connection dies.
func (w *SyncWorker) waitForTrigger(ctx context.Context, conn *pgxpool.Conn) (triggerType, error) {
	waitCtx, cancel := context.WithTimeout(ctx, w.cfg.PollInterval)
	defer cancel()

	notification, err := conn.Conn().WaitForNotification(waitCtx)

	switch {
	case err == nil:
		w.logger.Debug("received notification", "cluster_id", notification.Payload)
		return triggerNotification, nil

	case errors.Is(err, context.DeadlineExceeded):
		return triggerTimeout, nil

	case errors.Is(err, context.Canceled):
		return 0, fmt.Errorf("shutdown requested: %w", ctx.Err())

	case conn.Conn().IsClosed():
		return 0, fmt.Errorf("connection closed")

	default:
		w.logger.Warn("unexpected error waiting for notification", "error", err)
		return triggerTimeout, nil
	}
}

// IsReady returns true if the worker is connected and processing.
func (w *SyncWorker) IsReady() bool {
	return w.ready.Load()
}

func (w *SyncWorker) processAllPending(ctx context.Context) {
	for {
		processed, err := w.processOne(ctx)
		if err != nil {
			w.logger.Error("failed to process cluster", "error", err)
			time.Sleep(w.cfg.BackoffDelay)
			return
		}
		if !processed {
			return
		}
	}
}

func (w *SyncWorker) processOne(ctx context.Context) (bool, error) {
	// Context is cancelled on shutdown signal (SIGTERM/SIGINT).
	// This check prevents starting new work during graceful shutdown.
	if ctx.Err() != nil {
		return false, nil
	}

	// 1. Claim cluster
	cluster, err := w.claimCluster(ctx)
	if err != nil {
		return false, err
	}
	if cluster == nil {
		return false, nil
	}

	syncAction := dbconst.ClusterEventSyncAction_Sync
	if cluster.Deleted != nil {
		syncAction = dbconst.ClusterEventSyncAction_Delete
	}
	attempt := cluster.SyncAttempts + 1

	w.logger.Info("processing cluster",
		"cluster_id", cluster.ID,
		"name", cluster.Name,
		"organization", cluster.OrganizationName,
		"deleted", cluster.Deleted != nil,
		"action", syncAction,
		"attempt", attempt)

	// Create sync_claimed event for history (worker picked up the cluster)
	if _, err := w.queries.ClusterCreateSyncClaimedEvent(ctx, db.ClusterCreateSyncClaimedEventParams{
		ClusterID:  cluster.ID,
		SyncAction: pgtype.Text{String: string(syncAction), Valid: true},
		Attempt:    pgtype.Int4{Int32: attempt, Valid: true},
	}); err != nil {
		w.logger.Warn("failed to create sync_claimed event", "error", err)
	}

	projectName := gardener.ProjectName(cluster.OrganizationName)

	// 2. Ensure Gardener Project exists and get actual namespace
	namespace, err := w.gardener.EnsureProject(ctx, projectName, cluster.OrganizationID)
	if err != nil {
		w.logger.Error("failed to ensure gardener project",
			"project", projectName,
			"organization_id", cluster.OrganizationID,
			"error", err)
		// Mark as failed and continue (no event - transient infrastructure error)
		w.markSyncFailed(ctx, cluster.ID, "ensure project: "+err.Error(), nil)
		return true, nil
	}

	// If namespace is empty, project was just created and Gardener hasn't set the namespace yet
	if namespace == "" {
		w.logger.Info("project created but namespace not ready yet, will retry",
			"project", projectName)
		w.markSyncFailed(ctx, cluster.ID, "project namespace not ready yet", &syncFailedEvent{
			syncAction: syncAction,
			message:    "Waiting for organization namespace to be created",
			attempt:    attempt,
		})
		return true, nil
	}

	// 3. Generate shoot name (used only for creation, existing shoots are looked up by label)
	shootName := gardener.GenerateShootName(cluster.Name)

	clusterToSync := gardener.ClusterToSync{
		ID:                cluster.ID,
		OrganizationID:    cluster.OrganizationID,
		OrganizationName:  cluster.OrganizationName,
		Name:              cluster.Name,
		ShootName:         shootName,
		Namespace:         namespace,
		Region:            cluster.Region,
		KubernetesVersion: cluster.KubernetesVersion,
		Deleted:           cluster.Deleted,
		SyncAttempts:      int(cluster.SyncAttempts),
	}

	var syncErr error

	if syncAction == dbconst.ClusterEventSyncAction_Delete {
		syncErr = w.gardener.DeleteShootByClusterID(ctx, clusterToSync.ID)
	} else {
		syncErr = w.gardener.ApplyShoot(ctx, &clusterToSync)
	}

	// 4. Update status and create events
	if syncErr != nil {
		w.markSyncFailed(ctx, cluster.ID, syncErr.Error(), &syncFailedEvent{
			syncAction: syncAction,
			message:    syncErr.Error(),
			attempt:    attempt,
		})

		w.logger.Error("sync failed",
			"cluster_id", cluster.ID,
			"name", cluster.Name,
			"attempt", attempt,
			"max_attempts", w.cfg.MaxAttempts,
			"error", syncErr)

		return true, nil // Continue processing other clusters
	}

	err = w.queries.ClusterMarkSynced(ctx, db.ClusterMarkSyncedParams{
		ClusterID: cluster.ID,
	})
	if err != nil {
		return false, fmt.Errorf("mark synced: %w", err)
	}

	// Create sync_succeeded event (Gardener accepted the manifest)
	if _, err := w.queries.ClusterCreateSyncSucceededEvent(ctx, db.ClusterCreateSyncSucceededEventParams{
		ClusterID:  cluster.ID,
		SyncAction: pgtype.Text{String: string(syncAction), Valid: true},
		Message:    pgtype.Text{}, // NULL for success
	}); err != nil {
		w.logger.Warn("failed to create sync_succeeded event", "error", err)
	}

	w.logger.Info("synced cluster to gardener",
		"cluster_id", cluster.ID,
		"name", cluster.Name,
		"action", syncAction)

	return true, nil
}

// reconcileAll performs a full comparison between DB state and Gardener state
// to detect and fix any drift. This runs periodically as a safety net.
func (w *SyncWorker) reconcileAll(ctx context.Context) {
	// Skip reconciliation during shutdown - not worth starting a long operation
	if ctx.Err() != nil {
		return
	}

	w.logger.Info("starting full reconciliation")

	dbClusters, err := w.queries.ClusterListActive(ctx)
	if err != nil {
		w.logger.Error("failed to list clusters from DB", "error", err)
		return
	}

	shoots, err := w.gardener.ListShoots(ctx)
	if err != nil {
		w.logger.Error("failed to list shoots from Gardener", "error", err)
		return
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

	for _, cluster := range dbClusters {
		_, exists := shootByClusterID[cluster.ID]
		if !exists {
			w.logger.Warn("drift detected: shoot missing in Gardener",
				"cluster_id", cluster.ID, "name", cluster.Name)
			if err := w.queries.ClusterSyncReset(ctx, db.ClusterSyncResetParams{ClusterID: cluster.ID}); err != nil {
				w.logger.Error("failed to reset cluster synced", "error", err)
			}
			driftedClusterCount++
			continue
		}

	}

	// Orphaned Shoots (in Gardener but not in DB) are deleted
	for clusterID, shoot := range shootByClusterID {
		_, exists := dbClusterByID[clusterID]
		if !exists {
			w.logger.Warn("deleting orphaned shoot in Gardener",
				"shoot", shoot.Name, "cluster_id", clusterID)
			if err := w.gardener.DeleteShootByClusterID(ctx, clusterID); err != nil {
				w.logger.Error("failed to delete orphaned shoot",
					"shoot", shoot.Name, "error", err)
			}
		}
	}

	w.logger.Info("full reconciliation complete",
		"clusters", len(dbClusters),
		"shoots", len(shoots),
		"drift_detected", driftedClusterCount)

	if driftedClusterCount > 0 {
		w.processAllPending(ctx)
	}
}

// syncFailedEvent contains optional parameters for creating a sync_failed event.
// When nil, no event is created (useful for transient errors that don't need tracking).
type syncFailedEvent struct {
	syncAction dbconst.ClusterEventSyncAction
	message    string
	attempt    int32
}

// markSyncFailed marks a cluster as failed and optionally creates a sync_failed event.
func (w *SyncWorker) markSyncFailed(ctx context.Context, clusterID uuid.UUID, errMsg string, event *syncFailedEvent) {
	if err := w.queries.ClusterMarkSyncFailed(ctx, db.ClusterMarkSyncFailedParams{
		ClusterID: clusterID,
		Error:     pgtype.Text{String: errMsg, Valid: true},
	}); err != nil {
		w.logger.Error("failed to mark sync failed", "error", err)
	}

	if event != nil {
		if _, err := w.queries.ClusterCreateSyncFailedEvent(ctx, db.ClusterCreateSyncFailedEventParams{
			ClusterID:  clusterID,
			SyncAction: pgtype.Text{String: string(event.syncAction), Valid: true},
			Message:    pgtype.Text{String: event.message, Valid: true},
			Attempt:    pgtype.Int4{Int32: event.attempt, Valid: true},
		}); err != nil {
			w.logger.Warn("failed to create sync_failed event", "error", err)
		}
	}
}
