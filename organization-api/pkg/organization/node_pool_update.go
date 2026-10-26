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

func (s *Server) UpdateNodePool(
	ctx context.Context,
	req *organizationv1.UpdateNodePoolRequest,
) (*organizationv1.UpdateNodePoolResponse, error) {
	nodePoolID := uuid.MustParse(req.GetNodePoolId())

	if err := s.checkPermission(ctx, authz.CanEdit(), authz.NodePool(nodePoolID)); err != nil {
		return nil, err
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to begin transaction: %w", err))
	}
	defer rollback.Rollback(ctx, tx, s.logger)

	qtx := s.queries.WithTx(tx)

	// Every pool write locks the cluster, so it runs before or after a
	// SetNodePools of the same cluster, never in between.
	cluster, err := qtx.NodePoolLockCluster(ctx, db.NodePoolLockClusterParams{NodePoolID: nodePoolID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("node pool not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to lock cluster: %w", err))
	}

	// A live cluster keeps a pool that can run nodes (see node_pool_guard.go):
	// its last such pool cannot shrink to a maximum of 0.
	if req.GetAutoscaleMax() == 0 && !cluster.Deleted.Valid {
		others, err := otherActivePools(ctx, qtx, cluster.ID, pgtype.UUID{Bytes: nodePoolID, Valid: true})
		if err != nil {
			return nil, err
		}
		if others == 0 {
			return nil, failedPrecondition(msgNoActivePool)
		}
	}

	params := db.NodePoolUpdateParams{
		ID:           nodePoolID,
		AutoscaleMin: pgtype.Int4{Int32: req.GetAutoscaleMin(), Valid: true},
		AutoscaleMax: pgtype.Int4{Int32: req.GetAutoscaleMax(), Valid: true},
	}

	rowsAffected, err := qtx.NodePoolUpdate(ctx, params)
	if err != nil {
		// Lowering the maximum is never refused, even when the organization is
		// already over a quota that was lowered since.
		return nil, nodePoolWriteError(err, "", "", "update")
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("node pool not found"))
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit: %w", err))
	}

	s.logger.InfoContext(ctx, "node pool updated", "node_pool_id", nodePoolID)

	return organizationv1.UpdateNodePoolResponse_builder{}.Build(), nil
}
