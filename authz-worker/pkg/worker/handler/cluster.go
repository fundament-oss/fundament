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

// Cluster syncs a cluster's organization relationship to OpenFGA.
func (h *Handler) Cluster(ctx context.Context, qtx *db.Queries, clusterID uuid.UUID) error {
	cluster, err := qtx.GetClusterByID(ctx, db.GetClusterByIDParams{ID: clusterID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("cluster not found: %s", clusterID)
		}

		return fmt.Errorf("get cluster: %w", err)
	}

	h.logger.DebugContext(ctx, "handle cluster", "cluster", cluster)

	orgObj := authz.Organization(cluster.OrganizationID)
	clusterObj := authz.Cluster(cluster.ID)

	if cluster.Deleted.Valid {
		return h.deleteTuplesIfExist(ctx,
			tupleDelete(orgObj, authz.ActionOwner, clusterObj),
		)
	}

	return h.writeTuples(ctx, tuple(orgObj, authz.ActionOwner, clusterObj))
}
