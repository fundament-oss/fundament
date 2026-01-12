// Package worker implements the cluster sync worker.
// It listens for PostgreSQL notifications and syncs cluster state to Gardener.
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

	inFlight sync.WaitGroup // Track in-flight operations for graceful shutdown
	ready    atomic.Bool    // For health checks
}

// Config holds worker configuration.
type Config struct {
	PollInterval      time.Duration // Timeout for WaitForNotification (e.g., 30s)
	ReconcileInterval time.Duration // How often to run full reconciliation (e.g., 5m)
	MaxSyncAttempts   int32         // Max retries before giving up (e.g., 10)
}

// NewSyncWorker creates a new SyncWorker.
func NewSyncWorker(pool *pgxpool.Pool, gardenerClient gardener.Client, logger *slog.Logger, cfg Config) *SyncWorker {
	return &SyncWorker{
		pool:     pool,
		queries:  db.New(pool),
		gardener: gardenerClient,
		logger:   logger,
		cfg:      cfg,
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

// triggerType indicates what caused the event loop to wake up.
type triggerType int

const (
	triggerNotification triggerType = iota
	triggerTimeout
)

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

// reconcileAll performs a full comparison between DB state and Gardener state
// to detect and fix any drift. This runs periodically as a safety net.
func (w *SyncWorker) reconcileAll(ctx context.Context) {
	w.logger.Info("starting full reconciliation")

	// 1. Get all active clusters from DB
	dbClusters, err := w.queries.ListActiveClusters(ctx)
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
	dbClusterByID := make(map[uuid.UUID]db.ListActiveClustersRow)
	for _, c := range dbClusters {
		dbClusterByID[c.ID] = c
	}

	shootByClusterID := make(map[uuid.UUID]gardener.ShootInfo)
	for _, s := range shoots {
		if id, ok := s.Labels["fundament.io/cluster-id"]; ok {
			clusterID, err := uuid.Parse(id)
			if err == nil {
				shootByClusterID[clusterID] = s
			}
		}
	}

	// 4. Detect drift and mark for re-sync
	var driftCount int

	// Check for missing or outdated Shoots
	for _, cluster := range dbClusters {
		shoot, exists := shootByClusterID[cluster.ID]
		if !exists {
			w.logger.Warn("drift detected: shoot missing in Gardener",
				"cluster_id", cluster.ID, "name", cluster.Name)
			if err := w.queries.ResetClusterSynced(ctx, cluster.ID); err != nil {
				w.logger.Error("failed to reset cluster synced", "error", err)
			}
			driftCount++
			continue
		}

		// Compare key fields (expand as schema grows)
		expectedName := gardener.ShootName(cluster.OrganizationName, cluster.Name, w.gardener.MaxShootNameLength())
		if shoot.Name != expectedName {
			w.logger.Warn("drift detected: shoot name mismatch",
				"cluster_id", cluster.ID,
				"expected", expectedName,
				"got", shoot.Name)
			if err := w.queries.ResetClusterSynced(ctx, cluster.ID); err != nil {
				w.logger.Error("failed to reset cluster synced", "error", err)
			}
			driftCount++
		}
	}

	// Delete orphaned Shoots (in Gardener but not in DB)
	for clusterID, shoot := range shootByClusterID {
		if _, exists := dbClusterByID[clusterID]; !exists {
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
		"drift_detected", driftCount)

	// Process any newly-marked pending clusters
	if driftCount > 0 {
		w.processAllPending(ctx)
	}
}

func (w *SyncWorker) processOne(ctx context.Context) (bool, error) {
	w.inFlight.Add(1)
	defer w.inFlight.Done()

	// 1. Claim cluster in short transaction (releases lock immediately)
	cluster, err := w.claimCluster(ctx)
	if err != nil {
		return false, err
	}
	if cluster == nil {
		return false, nil // No work available
	}

	w.logger.Info("processing cluster",
		"cluster_id", cluster.ID,
		"name", cluster.Name,
		"organization", cluster.OrganizationName,
		"deleted", cluster.Deleted != nil,
		"attempt", cluster.SyncAttempts+1)

	// 2. Sync to Gardener (no DB lock held - allows other workers to proceed)
	var syncErr error
	clusterToSync := gardener.ClusterToSync{
		ID:                cluster.ID,
		Name:              cluster.Name,
		OrganizationName:  cluster.OrganizationName,
		Region:            cluster.Region,
		KubernetesVersion: cluster.KubernetesVersion,
		Deleted:           cluster.Deleted,
		SyncAttempts:      int(cluster.SyncAttempts),
	}

	if cluster.Deleted != nil {
		// Check if there's a new active cluster with the same name before deleting
		// This prevents deleting a shoot that's been recreated
		hasActive, err := w.queries.HasActiveClusterWithSameName(ctx, db.HasActiveClusterWithSameNameParams{
			Name:   cluster.OrganizationName,
			Name_2: cluster.Name,
		})
		if err != nil {
			return false, fmt.Errorf("check active cluster: %w", err)
		}
		if hasActive {
			w.logger.Info("skipping shoot deletion - active cluster with same name exists",
				"cluster_id", cluster.ID,
				"name", cluster.Name)
			// Mark as synced without actually deleting
			syncErr = nil
		} else {
			syncErr = w.gardener.DeleteShoot(ctx, &clusterToSync)
		}
	} else {
		syncErr = w.gardener.ApplyShoot(ctx, &clusterToSync)
	}

	// 3. Update status in new transaction
	if syncErr != nil {
		if err := w.queries.MarkClusterSyncFailed(ctx, db.MarkClusterSyncFailedParams{
			ClusterID: cluster.ID,
			SyncError: pgtype.Text{String: truncateError(syncErr.Error(), 1000), Valid: true},
		}); err != nil {
			w.logger.Error("failed to mark sync failed", "error", err)
		}

		attempts := cluster.SyncAttempts + 1
		w.logger.Error("sync failed",
			"cluster_id", cluster.ID,
			"name", cluster.Name,
			"attempt", attempts,
			"error", syncErr)

		if attempts >= w.cfg.MaxSyncAttempts {
			w.logger.Error("ALERT: cluster sync exhausted, will not retry",
				"cluster_id", cluster.ID,
				"name", cluster.Name,
				"attempts", attempts,
				"max_attempts", w.cfg.MaxSyncAttempts)
		}

		return true, nil // Continue processing other clusters
	}

	if err := w.queries.MarkClusterSynced(ctx, cluster.ID); err != nil {
		return false, fmt.Errorf("mark synced: %w", err)
	}

	action := "applied"
	if cluster.Deleted != nil {
		action = "deleted"
	}
	w.logger.Info("synced cluster to gardener",
		"cluster_id", cluster.ID,
		"name", cluster.Name,
		"action", action)

	return true, nil
}

// claimedCluster holds a claimed cluster's info.
type claimedCluster struct {
	ID                uuid.UUID
	Name              string
	OrganizationName  string
	Region            string
	KubernetesVersion string
	Deleted           *time.Time
	SyncAttempts      int32
}

// claimCluster atomically claims one unsynced cluster.
func (w *SyncWorker) claimCluster(ctx context.Context) (*claimedCluster, error) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row, err := w.queries.WithTx(tx).ClaimUnsyncedCluster(ctx, w.cfg.MaxSyncAttempts)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("claim cluster: %w", err)
	}

	// Commit immediately to release the lock
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit claim: %w", err)
	}

	// Convert pgtype to Go types
	var deleted *time.Time
	if row.Deleted.Valid {
		deleted = &row.Deleted.Time
	}

	return &claimedCluster{
		ID:                row.ID,
		Name:              row.Name,
		OrganizationName:  row.OrganizationName,
		Region:            row.Region,
		KubernetesVersion: row.KubernetesVersion,
		Deleted:           deleted,
		SyncAttempts:      row.SyncAttempts,
	}, nil
}

// truncateError limits error message length for DB storage.
func truncateError(msg string, maxLen int) string {
	if len(msg) <= maxLen {
		return msg
	}
	return msg[:maxLen-3] + "..."
}
