// Package inventory periodically records what runs on every ready cluster:
// nodes per worker pool, pods per fundament namespace and the cluster's
// allocatable capacity (FUN-23). It keeps the latest observation per cluster,
// node pool and namespace; the staff fleet view reads those snapshots.
//
// It runs apart from the status sweep on purpose. A shoot that cannot be read
// is recorded on its own snapshot and the loop carries on, so collection never
// takes the worker down and never affects readiness.
package inventory

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"k8s.io/client-go/util/workqueue"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/common/dbconst"
	"github.com/fundament-oss/fundament/common/kubename"
)

// maxErrorLength bounds the stored error. A Kubernetes error can quote a whole
// response body; the start says what went wrong.
const maxErrorLength = 1000

// recordErrorTimeout bounds storing a failed collection. It cannot use the
// collection's deadline, which may be what failed it, and without one of its
// own a stalled database would hold the worker, and its queue slot, forever.
const recordErrorTimeout = 10 * time.Second

// Config is read under the INVENTORY_ prefix.
type Config struct {
	// Interval is how often every ready cluster is queued for collection.
	Interval time.Duration `env:"INTERVAL" envDefault:"15m"`
	// Workers is how many clusters are collected at once.
	Workers int `env:"WORKERS" envDefault:"4"`
	// Timeout bounds one cluster's collection, credentials and storing
	// included.
	Timeout time.Duration `env:"TIMEOUT" envDefault:"30s"`
}

// Validate rejects settings the worker cannot run with: time.NewTicker panics
// on a non-positive interval, a heartbeat stores the interval in whole seconds
// (and requires at least one), and a non-positive timeout fails every
// collection.
func (c Config) Validate() error {
	if c.Interval < time.Second {
		return fmt.Errorf("INVENTORY_INTERVAL must be at least 1s, got %s", c.Interval)
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("INVENTORY_TIMEOUT must be positive, got %s", c.Timeout)
	}
	return nil
}

// Worker queues every ready cluster on each tick and collects them with a
// fixed number of workers. The queue never hands one cluster to two workers,
// and a cluster queued again before its turn is collected once.
type Worker struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	reader  shoot.InventoryReader
	logger  *slog.Logger
	cfg     Config
	queue   workqueue.TypedInterface[uuid.UUID]
	clock   func() time.Time
}

// New creates an inventory worker.
func New(pool *pgxpool.Pool, reader shoot.InventoryReader, logger *slog.Logger, cfg Config) *Worker {
	return &Worker{
		pool:    pool,
		queries: db.New(pool),
		reader:  reader,
		logger:  logger.With("component", "inventory"),
		cfg:     cfg,
		queue: workqueue.NewTypedWithConfig(workqueue.TypedQueueConfig[uuid.UUID]{
			Name: "cluster-inventory",
		}),
		clock: time.Now,
	}
}

// Run collects once at start and then every Interval until ctx is done. It
// only returns nil: a failed listing is logged and retried on the next tick.
func (w *Worker) Run(ctx context.Context) error {
	w.logger.Info("inventory worker starting",
		"interval", w.cfg.Interval,
		"workers", w.cfg.Workers,
		"timeout", w.cfg.Timeout)

	var wg sync.WaitGroup
	for range max(w.cfg.Workers, 1) {
		wg.Go(func() {
			for w.processNext(ctx) {
			}
		})
	}

	ticker := time.NewTicker(w.cfg.Interval)
	defer ticker.Stop()

	w.EnqueueAll(ctx)
	for {
		select {
		case <-ctx.Done():
			w.queue.ShutDown()
			wg.Wait()
			return nil
		case <-ticker.C:
			w.EnqueueAll(ctx)
		}
	}
}

// EnqueueAll queues every cluster that is due for collection and records the
// pass, so the fleet view can judge snapshots against the interval this worker
// actually runs at. The pass is recorded once the clusters are queued, not
// once they are collected.
func (w *Worker) EnqueueAll(ctx context.Context) {
	ids, err := w.queries.InventoryClusterList(ctx)
	if err != nil {
		if ctx.Err() == nil {
			w.logger.Error("failed to list clusters for inventory", "error", err)
		}
		return
	}
	for _, id := range ids {
		w.queue.Add(id)
	}
	w.logger.Debug("queued clusters for inventory", "clusters", len(ids))

	if err := w.queries.WorkerHeartbeatRecord(ctx, db.WorkerHeartbeatRecordParams{
		Worker:          string(dbconst.WorkerHeartbeatWorker_Inventory),
		IntervalSeconds: int32(w.cfg.Interval.Seconds()),
		LastPass:        pgtype.Timestamptz{Time: w.clock(), Valid: true},
	}); err != nil && ctx.Err() == nil {
		w.logger.Error("failed to record inventory pass", "error", err)
	}
}

func (w *Worker) processNext(ctx context.Context) bool {
	clusterID, shutdown := w.queue.Get()
	if shutdown {
		return false
	}
	defer w.queue.Done(clusterID)

	if err := w.Collect(ctx, clusterID); err != nil && ctx.Err() == nil {
		w.logger.Warn("inventory collection failed", "cluster_id", clusterID, "error", err)
	}
	return true
}

// Collect reads one cluster's shoot and stores the snapshot in a single
// transaction. A failed read or store lands in the cluster snapshot's
// last_error and is returned; the previous numbers stay. One Timeout covers
// both, so a slow shoot leaves less time to store. It bounds one collection
// only: how long a whole pass takes depends on the number of clusters and
// Workers.
func (w *Worker) Collect(ctx context.Context, clusterID uuid.UUID) error {
	collectCtx, cancel := context.WithTimeout(shoot.WithClientCache(ctx), w.cfg.Timeout)
	defer cancel()

	inv, err := w.reader.ReadInventory(collectCtx, clusterID)
	if err != nil {
		return w.recordError(ctx, clusterID, fmt.Errorf("read inventory: %w", err))
	}
	if err := w.store(collectCtx, clusterID, inv); err != nil {
		return w.recordError(ctx, clusterID, fmt.Errorf("store inventory: %w", err))
	}
	return nil
}

// recordError stores a failed collection on the cluster's snapshot. It runs on
// the worker's context, not the collection's, so a collection that ran out of
// time still records why, bounded by recordErrorTimeout.
func (w *Worker) recordError(ctx context.Context, clusterID uuid.UUID, err error) error {
	if ctx.Err() != nil {
		return nil //nolint:nilerr // shutting down: the collection was cancelled, not failed
	}
	ctx, cancel := context.WithTimeout(ctx, recordErrorTimeout)
	defer cancel()
	if recordErr := w.queries.ClusterSnapshotRecordError(ctx, db.ClusterSnapshotRecordErrorParams{
		ClusterID:   clusterID,
		LastError:   pgtype.Text{String: truncate(err.Error(), maxErrorLength), Valid: true},
		LastErrorAt: pgtype.Timestamptz{Time: w.clock(), Valid: true},
	}); recordErr != nil {
		return errors.Join(err, fmt.Errorf("record error: %w", recordErr))
	}
	return err
}

func (w *Worker) store(ctx context.Context, clusterID uuid.UUID, inv *shoot.Inventory) error {
	observed := pgtype.Timestamptz{Time: w.clock(), Valid: true}

	pools, err := w.queries.InventoryNodePoolList(ctx, db.InventoryNodePoolListParams{ClusterID: clusterID})
	if err != nil {
		return fmt.Errorf("list node pools: %w", err)
	}
	namespaces, err := w.queries.InventoryNamespaceList(ctx, db.InventoryNamespaceListParams{ClusterID: clusterID})
	if err != nil {
		return fmt.Errorf("list namespaces: %w", err)
	}

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := w.queries.WithTx(tx)

	if err := q.ClusterSnapshotUpsert(ctx, db.ClusterSnapshotUpsertParams{
		ClusterID:           clusterID,
		Observed:            observed,
		Nodes:               inv.Nodes,
		NodesReady:          inv.NodesReady,
		Pods:                inv.Pods,
		PodAllocatable:      inv.PodAllocatable,
		CpuAllocatableM:     inv.CPUAllocatableMilli,
		MemoryAllocatableMi: inv.MemoryAllocatableMi,
	}); err != nil {
		return fmt.Errorf("upsert cluster snapshot: %w", err)
	}

	// Every live pool gets a row: one Gardener runs no nodes for is
	// reported as zero, which is what the staff page flags below its minimum.
	if len(pools) > 0 {
		params := db.NodePoolSnapshotUpsertParams{Observed: observed}
		for _, p := range pools {
			counts := inv.Pools[kubename.GenerateWorkerName(p.Name)]
			params.NodePoolIds = append(params.NodePoolIds, p.ID)
			params.Nodes = append(params.Nodes, counts.Nodes)
			params.NodesReady = append(params.NodesReady, counts.NodesReady)
		}
		if err := q.NodePoolSnapshotUpsert(ctx, params); err != nil {
			return fmt.Errorf("upsert node pool snapshots: %w", err)
		}
	}

	// A live namespace missing on the shoot keeps its old snapshot, so its
	// observed time goes stale instead of reporting zero pods.
	params := db.NamespaceSnapshotUpsertParams{Observed: observed}
	for _, id := range namespaces {
		pods, ok := inv.NamespacePods[id]
		if !ok {
			continue
		}
		params.NamespaceIds = append(params.NamespaceIds, id)
		params.Pods = append(params.Pods, pods)
	}
	if len(params.NamespaceIds) > 0 {
		if err := q.NamespaceSnapshotUpsert(ctx, params); err != nil {
			return fmt.Errorf("upsert namespace snapshots: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// truncate cuts s to at most n bytes without splitting a UTF-8 sequence,
// which Postgres would reject.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}
