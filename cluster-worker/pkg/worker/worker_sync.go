// Package worker implements the cluster sync worker.
// It listens for PostgreSQL notifications and syncs cluster state to Gardener.
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

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

	inFlight sync.WaitGroup // Track in-flight operations for graceful shutdown
	ready    atomic.Bool    // For health checks
}

// Config holds worker configuration.
type Config struct {
	PollInterval      time.Duration `env:"POLL_INTERVAL" envDefault:"30s"`     // Timeout for WaitForNotification
	ReconcileInterval time.Duration `env:"RECONCILE_INTERVAL" envDefault:"5m"` // How often to run full reconciliation
	MaxAttempts       int32         `env:"MAX_ATTEMPTS" envDefault:"5"`        // Max retries before giving up
}

// triggerType indicates what caused the event loop to wake up.
type triggerType int

const (
	triggerNotification triggerType = iota
	triggerTimeout
)

// NewSyncWorker creates a new SyncWorker.
func NewSyncWorker(pool *pgxpool.Pool, gardenerClient gardener.Client, logger *slog.Logger, cfg Config) *SyncWorker {
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
		w.logger.Error("connection lost, reconnecting in 5s", "error", err)
		w.ready.Store(false)
		time.Sleep(5 * time.Second)
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

	// --- Main event loop ---
	for {
		// Wait for trigger: notification OR timeout (whichever comes first)
		_, err := w.waitForTrigger(ctx, conn)
		if err != nil {
			return err
		}

		// Process all pending clusters (may be none)
		w.processAllPending(ctx)

		// Periodic full reconciliation to detect drift
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
		// Notification received
		w.logger.Debug("received notification", "cluster_id", notification.Payload)
		return triggerNotification, nil

	case errors.Is(err, context.DeadlineExceeded):
		// PollInterval timeout - normal, just means no notifications
		return triggerTimeout, nil

	case errors.Is(err, context.Canceled) && ctx.Err() != nil:
		// Parent context canceled - shutdown requested
		return 0, fmt.Errorf("shutdown requested: %w", ctx.Err())

	case conn.Conn().IsClosed():
		// Connection died - caller should reconnect
		return 0, fmt.Errorf("connection closed")

	default:
		// Unexpected error - log and treat as timeout to continue processing
		w.logger.Warn("unexpected error waiting for notification", "error", err)
		return triggerTimeout, nil
	}
}

// Shutdown waits for in-flight operations to complete.
func (w *SyncWorker) Shutdown(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		w.inFlight.Wait()
		close(done)
	}()

	select {
	case <-done:
		w.logger.Info("graceful shutdown complete")
	case <-time.After(timeout):
		w.logger.Warn("shutdown timeout, some operations may be incomplete")
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
			time.Sleep(5 * time.Second) // Backoff on error
			return
		}
		if !processed {
			return // No more work
		}
	}
}

func (w *SyncWorker) processOne(ctx context.Context) (bool, error) {
	w.inFlight.Add(1)
	defer w.inFlight.Done()

	// 1. Claim cluster
	cluster, err := w.claimCluster(ctx)
	if err != nil {
		return false, err
	}
	if cluster == nil {
		return false, nil // No work available
	}

	// Determine sync action for events
	syncAction := db.TenantClusterSyncActionCreate
	if cluster.Deleted != nil {
		syncAction = db.TenantClusterSyncActionDelete
	} else if cluster.SyncAttempts > 0 {
		syncAction = db.TenantClusterSyncActionUpdate // Retry implies update
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
		SyncAction: db.NullTenantClusterSyncAction{TenantClusterSyncAction: syncAction, Valid: true},
		Attempt:    pgtype.Int4{Int32: attempt, Valid: true},
	}); err != nil {
		w.logger.Warn("failed to create sync_claimed event", "error", err)
	}

	// 2. Compute project name (deterministic from org name)
	projectName := gardener.ProjectName(cluster.OrganizationName)

	// 3. Ensure Gardener Project exists and get actual namespace
	namespace, err := w.gardener.EnsureProject(ctx, projectName, cluster.OrganizationID)
	if err != nil {
		w.logger.Error("failed to ensure gardener project",
			"project", projectName,
			"organization_id", cluster.OrganizationID,
			"error", err)
		// Mark as failed and continue
		if err := w.queries.ClusterMarkSyncFailed(ctx, db.ClusterMarkSyncFailedParams{
			ClusterID: cluster.ID,
			Error:     pgtype.Text{String: truncateError("ensure project: "+err.Error(), 1000), Valid: true},
		}); err != nil {
			w.logger.Error("failed to mark sync failed", "error", err)
		}
		return true, nil
	}

	// If namespace is empty, project was just created and Gardener hasn't set the namespace yet
	if namespace == "" {
		w.logger.Info("project created but namespace not ready yet, will retry",
			"project", projectName)
		if err := w.queries.ClusterMarkSyncFailed(ctx, db.ClusterMarkSyncFailedParams{
			ClusterID: cluster.ID,
			Error:     pgtype.Text{String: "project namespace not ready yet", Valid: true},
		}); err != nil {
			w.logger.Error("failed to mark sync failed", "error", err)
		}
		// Create sync_failed event to track namespace wait
		if _, err := w.queries.ClusterCreateSyncFailedEvent(ctx, db.ClusterCreateSyncFailedEventParams{
			ClusterID:  cluster.ID,
			SyncAction: db.NullTenantClusterSyncAction{TenantClusterSyncAction: syncAction, Valid: true},
			Message:    pgtype.Text{String: "Waiting for organization namespace to be created", Valid: true},
			Attempt:    pgtype.Int4{Int32: attempt, Valid: true},
		}); err != nil {
			w.logger.Warn("failed to create sync_failed event", "error", err)
		}
		return true, nil
	}

	// 4. Generate shoot name (used only for creation, existing shoots are looked up by label)
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

	if syncAction == db.TenantClusterSyncActionDelete {
		syncErr = w.gardener.DeleteShoot(ctx, &clusterToSync)
	} else {
		syncErr = w.gardener.ApplyShoot(ctx, &clusterToSync)
	}

	// 3. Update status and create events

	// If failed update the cluster status and add event
	if syncErr != nil {
		if err := w.queries.ClusterMarkSyncFailed(ctx, db.ClusterMarkSyncFailedParams{
			ClusterID: cluster.ID,
			Error:     pgtype.Text{String: truncateError(syncErr.Error(), 1000), Valid: true},
		}); err != nil {
			w.logger.Error("failed to mark sync failed", "error", err)
		}

		// Create sync_failed event
		if _, err := w.queries.ClusterCreateSyncFailedEvent(ctx, db.ClusterCreateSyncFailedEventParams{
			ClusterID:  cluster.ID,
			SyncAction: db.NullTenantClusterSyncAction{TenantClusterSyncAction: syncAction, Valid: true},
			Message:    pgtype.Text{String: truncateError(syncErr.Error(), 1000), Valid: true},
			Attempt:    pgtype.Int4{Int32: attempt, Valid: true},
		}); err != nil {
			w.logger.Warn("failed to create sync_failed event", "error", err)
		}

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

	// Create sync_submitted event (Gardener accepted the manifest)
	if _, err := w.queries.ClusterCreateSyncSubmittedEvent(ctx, db.ClusterCreateSyncSubmittedEventParams{
		ClusterID:  cluster.ID,
		SyncAction: db.NullTenantClusterSyncAction{TenantClusterSyncAction: syncAction, Valid: true},
		Message:    pgtype.Text{}, // NULL for success
	}); err != nil {
		w.logger.Warn("failed to create sync_submitted event", "error", err)
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
	w.logger.Info("starting full reconciliation")

	// 1. Get all active clusters from DB
	dbClusters, err := w.queries.ClusterListActive(ctx)
	if err != nil {
		w.logger.Error("failed to list clusters from DB", "error", err)
		return
	}

	// 2. Get all Shoots from Gardener (filtered by our labels)
	shoots, err := w.gardener.ListShoots(ctx)
	if err != nil {
		w.logger.Error("failed to list shoots from Gardener", "error", err)
		return
	}

	// 3. Build lookup maps
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

	// 4. Detect drift and mark for re-sync
	var driftedClusterCount int

	// Check for missing or outdated Shoots
	for _, cluster := range dbClusters {
		shoot, exists := shootByClusterID[cluster.ID]
		// Missing cluster
		if !exists {
			w.logger.Warn("drift detected: shoot missing in Gardener",
				"cluster_id", cluster.ID, "name", cluster.Name)
			if err := w.queries.ClusterSyncReset(ctx, db.ClusterSyncResetParams{ClusterID: cluster.ID}); err != nil {
				w.logger.Error("failed to reset cluster synced", "error", err)
			}
			driftedClusterCount++
			continue
		}

		// Outdated - check if key fields have drifted
		hasDrifted, drifted := w.shootKeyFieldsDrifted(&cluster, shoot)
		if hasDrifted {
			w.logger.Warn("drift detected: ",
				"cluster_id", cluster.ID,
				"name", cluster.Name,
				"drifted", drifted,
			)
			if err := w.queries.ClusterSyncReset(ctx, db.ClusterSyncResetParams{ClusterID: cluster.ID}); err != nil {
				w.logger.Error("failed to reset cluster synced", "error", err)
			}
			driftedClusterCount++
		}
	}

	// Orphaned Shoots (in Gardener but not in DB) are deleted
	for clusterID, shoot := range shootByClusterID {
		_, exists := dbClusterByID[clusterID]
		if !exists {
			w.logger.Warn("deleting orphaned shoot in Gardener",
				"shoot", shoot.Name, "cluster_id", clusterID)
			if err := w.gardener.DeleteShootByName(ctx, shoot.Name); err != nil {
				w.logger.Error("failed to delete orphaned shoot",
					"shoot", shoot.Name, "error", err)
			}
		}
	}

	w.logger.Info("full reconciliation complete",
		"clusters", len(dbClusters),
		"shoots", len(shoots),
		"drift_detected", driftedClusterCount)

	// Process any newly-marked pending clusters
	if driftedClusterCount > 0 {
		w.processAllPending(ctx)
	}
}

// shootKeyFieldsDrifted checks if a Shoot's key fields differ from the expected cluster state.
// Returns a description of the drift, or empty string if no drift detected.
// With label-based lookup, drift detection is based on shoot existence (handled elsewhere).
// Expand this function as the schema grows to include more fields (region, k8s version, etc).
func (w *SyncWorker) shootKeyFieldsDrifted(cluster *db.ClusterListActiveRow, shoot gardener.ShootInfo) (bool, []string) {
	// Currently no additional drift detection beyond existence check
	// The reconciliation loop already handles missing shoots via label-based lookup
	return false, nil
}

// truncateError limits error message length for DB storage.
func truncateError(msg string, maxLen int) string {
	if len(msg) <= maxLen {
		return msg
	}
	return msg[:maxLen-3] + "..."
}
