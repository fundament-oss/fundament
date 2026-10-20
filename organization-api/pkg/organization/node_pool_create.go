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

func (s *Server) CreateNodePool(
	ctx context.Context,
	req *organizationv1.CreateNodePoolRequest,
) (*organizationv1.CreateNodePoolResponse, error) {
	clusterID := uuid.MustParse(req.GetClusterId())

	// Retry: the add-cluster wizard adds node pools immediately after creating
	// the cluster, before its authz tuple has synced (see checkPermissionWithRetry).
	if err := s.checkPermissionWithRetry(ctx, authz.CanCreateNodePool(), authz.Cluster(clusterID)); err != nil {
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
	locked, err := qtx.ClusterLockForNodePools(ctx, db.ClusterLockForNodePoolsParams{ClusterID: clusterID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to lock cluster: %w", err))
	}

	if locked.Deleted.Valid {
		return nil, failedPrecondition(msgClusterDeleting)
	}

	// Resolve the machine type name against the catalog within the cluster's
	// region: only offered types are valid (pre-validate with ListRegions).
	offering, err := qtx.RegionMachineTypeResolve(ctx, db.RegionMachineTypeResolveParams{
		RegionName:      locked.Region,
		MachineTypeName: req.GetMachineType(),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("machine type %q is not offered in region %q", req.GetMachineType(), locked.Region))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to resolve machine type: %w", err))
	}

	params := db.NodePoolCreateParams{
		ClusterID:           clusterID,
		Name:                req.GetName(),
		MachineType:         req.GetMachineType(),
		RegionMachineTypeID: pgtype.UUID{Bytes: offering, Valid: true},
		AutoscaleMin:        req.GetAutoscaleMin(),
		AutoscaleMax:        req.GetAutoscaleMax(),
	}

	// A live cluster keeps a pool that can run nodes (see node_pool_guard.go):
	// a pool with a maximum of 0 cannot be its first.
	if req.GetAutoscaleMax() == 0 {
		active, err := otherActivePools(ctx, qtx, clusterID, pgtype.UUID{})
		if err != nil {
			return nil, err
		}
		if active == 0 {
			return nil, failedPrecondition(msgFirstPoolEmpty)
		}
	}

	nodePoolID, err := qtx.NodePoolCreate(ctx, params)
	if err != nil {
		return nil, nodePoolWriteError(err, req.GetMachineType(), locked.Region, "create")
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit: %w", err))
	}

	s.logger.InfoContext(ctx, "node pool created",
		"node_pool_id", nodePoolID,
		"cluster_id", clusterID,
		"name", req.GetName(),
	)

	return organizationv1.CreateNodePoolResponse_builder{
		NodePoolId: nodePoolID.String(),
	}.Build(), nil
}
