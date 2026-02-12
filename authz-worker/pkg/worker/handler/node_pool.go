package handler

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	db "github.com/fundament-oss/fundament/authz-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/common/authz"
)

// NodePool syncs a node pool's cluster relationship to OpenFGA.
func (h *Handler) NodePool(ctx context.Context, qtx *db.Queries, nodePoolID uuid.UUID) error {
	nodePool, err := qtx.GetNodePoolByID(ctx, db.GetNodePoolByIDParams{ID: nodePoolID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("node pool not found: %s", nodePoolID)
		}

		return fmt.Errorf("get node pool: %w", err)
	}

	h.logger.DebugContext(ctx, "handle node_pool", "node_pool", nodePool)

	clusterObj := authz.Cluster(nodePool.ClusterID)
	nodePoolObj := authz.NodePool(nodePool.ID)

	if nodePool.Deleted.Valid {
		return h.deleteTuplesIfExist(ctx,
			tupleDelete(clusterObj, authz.ActionParent, nodePoolObj),
		)
	}

	return h.writeTuples(ctx, tuple(clusterObj, authz.ActionParent, nodePoolObj))
}
