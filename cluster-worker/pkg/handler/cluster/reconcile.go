package cluster

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
)

// Reconcile compares DB state with Gardener state to detect drift and to
// finish deleting the shoots of deleted clusters.
func (h *Handler) Reconcile(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil //nolint:nilerr // graceful shutdown
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

	// Drift detection: clusters missing in Gardener. The insert decides whether
	// one is repaired (its latest sync completed and nothing is queued).
	var driftedCount int
	for _, cluster := range dbClusters {
		_, exists := shootByClusterID[cluster.ID]
		if exists {
			continue
		}
		queued, err := h.queries.OutboxInsertReconcile(ctx, db.OutboxInsertReconcileParams{
			ClusterID: pgtype.UUID{Bytes: cluster.ID, Valid: true},
		})
		if err != nil {
			h.logger.Error("failed to insert reconcile outbox row", "cluster_id", cluster.ID, "error", err)
			continue
		}
		if queued == 0 {
			continue
		}
		h.logger.Warn("drift detected: shoot missing in Gardener",
			"cluster_id", cluster.ID, "name", cluster.Name)
		driftedCount++
	}

	unknownCount, err := h.cleanupOrphans(ctx, dbClusterByID, shootByClusterID)

	h.logger.Info("cluster reconciliation complete",
		"clusters", len(dbClusters),
		"shoots", len(shoots),
		"drift_detected", driftedCount,
		"unknown_shoots", unknownCount)

	return err
}

// cleanupOrphans handles shoots in Gardener without an active cluster. Only a
// shoot whose cluster row exists and is soft-deleted is deleted: a user asked
// for that and the delete sync did not finish it. A shoot without a cluster
// row is reported and left alone, since an empty, restored or wrong database
// or another installation sharing this Gardener looks exactly the same.
// Returns the number of shoots without a cluster row.
func (h *Handler) cleanupOrphans(ctx context.Context, dbClusterByID map[uuid.UUID]db.ClusterListActiveRow, shootByClusterID map[uuid.UUID]gardener.ShootInfo) (int, error) {
	var orphanIDs []uuid.UUID
	for clusterID := range shootByClusterID {
		if _, exists := dbClusterByID[clusterID]; !exists {
			orphanIDs = append(orphanIDs, clusterID)
		}
	}
	if len(orphanIDs) == 0 {
		return 0, nil
	}

	deletedClusters, err := h.queries.ClusterListDeletedByIDs(ctx, db.ClusterListDeletedByIDsParams{ClusterIds: orphanIDs})
	if err != nil {
		h.logger.Error("failed to list deleted clusters from DB", "error", err)
		return 0, fmt.Errorf("list deleted clusters: %w", err)
	}

	deleted := make(map[uuid.UUID]bool, len(deletedClusters))
	// Shoots already being deleted are only confirmed again: their deletion was
	// started before, and repeating it only adds the confirmations Gardener may
	// still need. They do not count towards the limit.
	var toConfirm, toStart []uuid.UUID
	for _, c := range deletedClusters {
		deleted[c.ID] = true
		// The delete sync is still working on this cluster; it deletes the
		// shoot itself, or settles as failed and leaves it to a later pass.
		if c.SyncRunning {
			continue
		}
		if shootByClusterID[c.ID].Deleting {
			toConfirm = append(toConfirm, c.ID)
		} else {
			toStart = append(toStart, c.ID)
		}
	}

	var unknownCount int
	for _, clusterID := range orphanIDs {
		if !deleted[clusterID] {
			shoot := shootByClusterID[clusterID]
			h.logger.Warn("shoot has no cluster in the database, not deleting it",
				"shoot", shoot.Name, "namespace", shoot.Namespace, "cluster_id", clusterID)
			unknownCount++
		}
	}

	// Refusing does not fail the pass: the reconcile worker exits after a few
	// failed passes, which would stop the outbox and status workers as well.
	if len(toStart) > h.cfg.MaxReconcileDeletions {
		h.logger.Error("refusing to delete shoots: too many in one pass, check the database",
			"shoots_to_delete", len(toStart),
			"max", h.cfg.MaxReconcileDeletions,
			"cluster_ids", toStart)
		toStart = nil
	}

	for _, clusterID := range slices.Concat(toConfirm, toStart) {
		shoot := shootByClusterID[clusterID]
		h.logger.Warn("deleting shoot of deleted cluster in Gardener",
			"shoot", shoot.Name, "cluster_id", clusterID)
		if err := h.gardener.DeleteShootByClusterID(ctx, clusterID); err != nil {
			h.logger.Error("failed to delete shoot of deleted cluster",
				"shoot", shoot.Name, "error", err)
		}
	}

	return unknownCount, nil
}
