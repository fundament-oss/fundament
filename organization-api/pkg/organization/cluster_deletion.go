package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
)

// A deleted cluster stays readable while Gardener tears it down, with status
// DELETING, so a caller can follow the deletion on the cluster itself. Once
// Gardener confirms the Shoot is gone the cluster is not found, and the
// authz-worker removes its tuple; from then on the permission check refuses
// first, which these helpers report as not found too.

// clusterDeletionConfirmed reports whether a cluster row is deleted and
// Gardener confirmed its Shoot is gone.
func clusterDeletionConfirmed(deleted pgtype.Timestamptz, shootStatus pgtype.Text) bool {
	return deleted.Valid && shootStatus.Valid && shootStatus.String == "deleted"
}

// clusterGoneOr returns not found for a permission error on a cluster whose
// deletion Gardener confirmed, and err otherwise.
func (s *Server) clusterGoneOr(ctx context.Context, clusterID uuid.UUID, err error) error {
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		return err
	}
	gone, qerr := s.queries.ClusterDeletionConfirmed(ctx, db.ClusterDeletionConfirmedParams{ClusterID: clusterID})
	if qerr != nil {
		s.logger.WarnContext(ctx, "check cluster deletion", "cluster_id", clusterID, "error", qerr)
		return err
	}
	if !gone {
		return err
	}
	return connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
}

// clusterWriteMissed tells why a write on a cluster changed no row: a cluster
// being deleted takes no more changes, which the caller hears as such rather
// than as not found for a cluster GetCluster still returns.
func (s *Server) clusterWriteMissed(ctx context.Context, clusterID uuid.UUID) error {
	cluster, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{ID: clusterID})
	if err == nil && cluster.Deleted.Valid && !clusterDeletionConfirmed(cluster.Deleted, cluster.ShootStatus) {
		return failedPrecondition(msgClusterDeleting)
	}
	return connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
}

// nodePoolGoneOr is clusterGoneOr for a node pool, whose permissions come from
// its cluster.
func (s *Server) nodePoolGoneOr(ctx context.Context, nodePoolID uuid.UUID, err error) error {
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		return err
	}
	gone, qerr := s.queries.NodePoolClusterDeletionConfirmed(ctx, db.NodePoolClusterDeletionConfirmedParams{NodePoolID: nodePoolID})
	if qerr != nil {
		s.logger.WarnContext(ctx, "check node pool cluster deletion", "node_pool_id", nodePoolID, "error", qerr)
		return err
	}
	if !gone {
		return err
	}
	return connect.NewError(connect.CodeNotFound, fmt.Errorf("node pool not found"))
}
