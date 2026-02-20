package cluster

import (
	"context"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
)

// ReconcileOrphans compares shoots in Gardener against active clusters in the DB.
// Shoots whose cluster ID doesn't exist or is soft-deleted are cleaned up.
func (h *Handler) ReconcileOrphans(ctx context.Context) error {
	shoots, err := h.gardener.ListShoots(ctx)
	if err != nil {
		return err
	}
	if len(shoots) == 0 {
		return nil
	}

	activeIDs, err := h.queries.ClusterListActiveIDs(ctx)
	if err != nil {
		return err
	}

	active := make(map[uuid.UUID]struct{}, len(activeIDs))
	for _, id := range activeIDs {
		active[id] = struct{}{}
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

		if _, ok := active[clusterID]; ok {
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
