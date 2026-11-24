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

// shootStatusDeleted is the shoot status cluster-worker writes once Gardener
// confirms a deleted cluster's Shoot is gone (or it never had one).
const shootStatusDeleted = "deleted"

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

	// A deleted cluster stays readable while Gardener tears it down, so callers
	// can follow the deletion; the tuple goes once the Shoot is confirmed gone.
	// The status update that confirms it queues the cluster here again.
	if clusterAccessEnded(&cluster) {
		return h.deleteTuplesIfExist(ctx,
			tupleDelete(orgObj, authz.ActionOwner, clusterObj),
		)
	}

	return h.writeTuplesIfNotExist(ctx, tuple(orgObj, authz.ActionOwner, clusterObj))
}

func clusterAccessEnded(cluster *db.GetClusterByIDRow) bool {
	return cluster.Deleted.Valid && cluster.ShootStatus.Valid && cluster.ShootStatus.String == shootStatusDeleted
}
