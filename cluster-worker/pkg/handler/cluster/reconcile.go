package cluster

import (
	"context"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
)

// ReconcileOrphans compares shoots in Gardener against all known clusters in the DB.
// Shoots whose cluster ID doesn't exist in the DB at all are deleted.
// Soft-deleted clusters are NOT treated as orphans — their shoots are being cleaned
// up by the normal outbox delete flow and the status worker.
func (h *Handler) ReconcileOrphans(ctx context.Context) error {
	shoots, err := h.gardener.ListShoots(ctx)
	if err != nil {
		return err
	}
	if len(shoots) == 0 {
		return nil
	}

	allIDs, err := h.queries.ClusterListAllIDs(ctx)
	if err != nil {
		return err
	}

	known := make(map[uuid.UUID]struct{}, len(allIDs))
	for _, id := range allIDs {
		known[id] = struct{}{}
	}

	for _, shoot := range shoots {
		clusterID := shoot.ClusterID
		if clusterID == uuid.Nil {
			if raw, ok := shoot.Labels[gardener.LabelClusterID]; ok {
				clusterID, _ = uuid.Parse(raw)
			}
		}
		if clusterID == uuid.Nil {
			h.logger.Warn("orphan detection: shoot has no cluster ID label, skipping",
				"shoot", shoot.Name,
				"namespace", shoot.Namespace)
			continue
		}

		if _, ok := known[clusterID]; ok {
			continue
		}

		h.logger.Info("deleting orphaned shoot",
			"shoot", shoot.Name,
			"cluster_id", clusterID)

		if err := h.gardener.DeleteShootByClusterID(ctx, clusterID); err != nil {
			h.logger.Error("failed to delete orphaned shoot",
				"shoot", shoot.Name,
				"cluster_id", clusterID,
				"error", err)
		}
	}

	return nil
}
