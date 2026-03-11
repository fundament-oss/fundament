package cluster

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
)

// Reconcile compares DB state with Gardener state to detect drift and clean up orphans.
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
