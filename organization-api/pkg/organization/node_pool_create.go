package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) CreateNodePool(
	ctx context.Context,
	req *connect.Request[organizationv1.CreateNodePoolRequest],
) (*connect.Response[organizationv1.CreateNodePoolResponse], error) {
	clusterID := uuid.MustParse(req.Msg.GetClusterId())

	// Retry: the add-cluster wizard adds node pools immediately after creating
	// the cluster, before its authz tuple has synced (see checkPermissionWithRetry).
	if err := s.checkPermissionWithRetry(ctx, authz.CanCreateNodePool(), authz.Cluster(clusterID)); err != nil {
		return nil, err
	}

	cluster, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{ID: clusterID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
	}

	// Resolve the machine type name against the catalog within the cluster's
	// region: only offered types are valid (pre-validate with ListRegions).
	offering, err := s.queries.RegionMachineTypeResolve(ctx, db.RegionMachineTypeResolveParams{
		RegionName:      cluster.Region,
		MachineTypeName: req.Msg.GetMachineType(),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("machine type %q is not offered in region %q", req.Msg.GetMachineType(), cluster.Region))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to resolve machine type: %w", err))
	}

	params := db.NodePoolCreateParams{
		ClusterID:           clusterID,
		Name:                req.Msg.GetName(),
		MachineType:         req.Msg.GetMachineType(),
		RegionMachineTypeID: pgtype.UUID{Bytes: offering, Valid: true},
		AutoscaleMin:        req.Msg.GetAutoscaleMin(),
		AutoscaleMax:        req.Msg.GetAutoscaleMax(),
	}

	nodePoolID, err := s.queries.NodePoolCreate(ctx, params)
	if err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
			// The region-match constraint trigger: the resolved row's region does
			// not match the cluster's region_id (e.g. a pre-catalog cluster whose
			// region_id is still NULL).
			if pgErr.Code == pgerrcode.RaiseException && pgErr.Hint == dbconst.HintNodePoolRegionMismatch {
				return nil, connect.NewError(connect.CodeFailedPrecondition,
					fmt.Errorf("the cluster is not linked to the region catalog; contact an operator"))
			}
			if pgErr.Code == pgerrcode.ForeignKeyViolation && pgErr.ConstraintName == dbconst.ConstraintNodePoolsFkRegionMachineType {
				return nil, connect.NewError(connect.CodeInvalidArgument,
					fmt.Errorf("machine type %q is not offered in region %q", req.Msg.GetMachineType(), cluster.Region))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create node pool: %w", err))
	}

	s.logger.InfoContext(ctx, "node pool created",
		"node_pool_id", nodePoolID,
		"cluster_id", clusterID,
		"name", req.Msg.GetName(),
	)

	return connect.NewResponse(organizationv1.CreateNodePoolResponse_builder{
		NodePoolId: nodePoolID.String(),
	}.Build()), nil
}
