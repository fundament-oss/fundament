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

	if _, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{ID: clusterID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
	}

	params := db.NodePoolCreateParams{
		ClusterID:    clusterID,
		Name:         req.Msg.GetName(),
		MachineType:  req.Msg.GetMachineType(),
		AutoscaleMin: req.Msg.GetAutoscaleMin(),
		AutoscaleMax: req.Msg.GetAutoscaleMax(),
	}

	// Catalog path: resolve the region_machine_types row and fill the legacy
	// machine_type text column from it (expand phase - the worker still reads
	// the text column). The region-match trigger asserts the row's region is
	// the cluster's region. Legacy path: the text field is stored as-is.
	if req.Msg.GetRegionMachineTypeId() != "" {
		regionMachineTypeID, err := parseUUIDField(req.Msg.GetRegionMachineTypeId(), "region_machine_type_id")
		if err != nil {
			return nil, err
		}

		offering, err := s.queries.RegionMachineTypeGet(ctx, db.RegionMachineTypeGetParams{ID: regionMachineTypeID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unknown machine type"))
			}
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to resolve machine type: %w", err))
		}

		params.MachineType = offering.MachineTypeName
		params.RegionMachineTypeID = pgtype.UUID{Bytes: regionMachineTypeID, Valid: true}
	} else if req.Msg.GetMachineType() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("either region_machine_type_id or machine_type must be set"))
	}

	nodePoolID, err := s.queries.NodePoolCreate(ctx, params)
	if err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
			// The region-match constraint trigger: the machine type belongs to a
			// different region than the cluster (also raised when the cluster
			// predates the catalog and has no region_id yet).
			if pgErr.Code == pgerrcode.RaiseException && pgErr.Hint == dbconst.HintNodePoolRegionMismatch {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("the machine type is not offered in the cluster's region"))
			}
			if pgErr.Code == pgerrcode.ForeignKeyViolation && pgErr.ConstraintName == dbconst.ConstraintNodePoolsFkRegionMachineType {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unknown machine type"))
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
