package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/rollback"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) DeleteNodePool(
	ctx context.Context,
	req *organizationv1.DeleteNodePoolRequest,
) (*organizationv1.DeleteNodePoolResponse, error) {
	nodePoolID := uuid.MustParse(req.GetNodePoolId())

	if err := s.checkPermission(ctx, authz.CanDelete(), authz.NodePool(nodePoolID)); err != nil {
		return nil, err
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to begin transaction: %w", err))
	}
	defer rollback.Rollback(ctx, tx, s.logger)

	qtx := s.queries.WithTx(tx)

	// A live cluster keeps a pool that can run nodes (see node_pool_guard.go),
	// so its last such pool stays. The cluster row is locked first, so parallel
	// deletes of its last two pools cannot both pass.
	cluster, err := qtx.NodePoolLockCluster(ctx, db.NodePoolLockClusterParams{NodePoolID: nodePoolID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("node pool not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to lock cluster: %w", err))
	}

	if !cluster.Deleted.Valid {
		others, err := otherActivePools(ctx, qtx, cluster.ID, pgtype.UUID{Bytes: nodePoolID, Valid: true})
		if err != nil {
			return nil, err
		}
		if others == 0 {
			// Other pools that cannot run nodes do not help; say which it is.
			live, err := qtx.NodePoolCountByClusterID(ctx, db.NodePoolCountByClusterIDParams{
				ClusterID:        cluster.ID,
				ExceptNodePoolID: pgtype.UUID{Bytes: nodePoolID, Valid: true},
			})
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to count node pools: %w", err))
			}
			if live > 0 {
				return nil, failedPrecondition(msgNoActivePool)
			}
			return nil, failedPrecondition(msgLastNodePool)
		}
	}

	rowsAffected, err := qtx.NodePoolDelete(ctx, db.NodePoolDeleteParams{ID: nodePoolID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete node pool: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("node pool not found"))
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit: %w", err))
	}

	s.logger.InfoContext(ctx, "node pool deleted", "node_pool_id", nodePoolID)

	return organizationv1.DeleteNodePoolResponse_builder{}.Build(), nil
}
