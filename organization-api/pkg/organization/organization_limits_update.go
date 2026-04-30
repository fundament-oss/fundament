package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) UpdateOrganizationLimits(
	ctx context.Context,
	req *connect.Request[organizationv1.UpdateOrganizationLimitsRequest],
) (*connect.Response[organizationv1.UpdateOrganizationLimitsResponse], error) {
	organizationID := uuid.MustParse(req.Msg.GetId())

	if err := s.checkPermission(ctx, authz.CanEdit(), authz.Organization(organizationID)); err != nil {
		return nil, err
	}

	params := db.OrganizationLimitsUpsertParams{
		OrganizationID:         organizationID,
		MaxNodesPerCluster:     pgtype.Int4{Int32: req.Msg.GetMaxNodesPerCluster(), Valid: req.Msg.HasMaxNodesPerCluster()},
		MaxNodePoolsPerCluster: pgtype.Int4{Int32: req.Msg.GetMaxNodePoolsPerCluster(), Valid: req.Msg.HasMaxNodePoolsPerCluster()},
		MaxNodesPerNodePool:    pgtype.Int4{Int32: req.Msg.GetMaxNodesPerNodePool(), Valid: req.Msg.HasMaxNodesPerNodePool()},
		DefaultMemoryRequestMi: pgtype.Int4{Int32: req.Msg.GetDefaultMemoryRequestMi(), Valid: req.Msg.HasDefaultMemoryRequestMi()},
		DefaultMemoryLimitMi:   pgtype.Int4{Int32: req.Msg.GetDefaultMemoryLimitMi(), Valid: req.Msg.HasDefaultMemoryLimitMi()},
		DefaultCpuRequestM:     pgtype.Int4{Int32: req.Msg.GetDefaultCpuRequestM(), Valid: req.Msg.HasDefaultCpuRequestM()},
		DefaultCpuLimitM:       pgtype.Int4{Int32: req.Msg.GetDefaultCpuLimitM(), Valid: req.Msg.HasDefaultCpuLimitM()},
	}

	if _, err := s.queries.OrganizationLimitsUpsert(ctx, params); err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == pgerrcode.CheckViolation {
			switch pgErr.ConstraintName {
			case dbconst.ConstraintOrganizationLimitsCkMemoryLimitGteRequest:
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("memory limit must be greater than or equal to memory request"))
			case dbconst.ConstraintOrganizationLimitsCkCpuLimitGteRequest:
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("CPU limit must be greater than or equal to CPU request"))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update organization limits: %w", err))
	}

	s.logger.InfoContext(ctx, "organization limits updated", "organization_id", organizationID)

	return connect.NewResponse(organizationv1.UpdateOrganizationLimitsResponse_builder{}.Build()), nil
}
