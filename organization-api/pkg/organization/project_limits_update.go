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

func (s *Server) UpdateProjectLimits(
	ctx context.Context,
	req *connect.Request[organizationv1.UpdateProjectLimitsRequest],
) (*connect.Response[organizationv1.UpdateProjectLimitsResponse], error) {
	projectID := uuid.MustParse(req.Msg.GetProjectId())

	if err := s.checkPermission(ctx, authz.CanEdit(), authz.Project(projectID)); err != nil {
		return nil, err
	}

	params := db.ProjectLimitsUpsertParams{
		ProjectID:              projectID,
		DefaultMemoryRequestMi: pgtype.Int4{Int32: req.Msg.GetDefaultMemoryRequestMi(), Valid: req.Msg.HasDefaultMemoryRequestMi()},
		DefaultMemoryLimitMi:   pgtype.Int4{Int32: req.Msg.GetDefaultMemoryLimitMi(), Valid: req.Msg.HasDefaultMemoryLimitMi()},
		DefaultCpuRequestM:     pgtype.Int4{Int32: req.Msg.GetDefaultCpuRequestM(), Valid: req.Msg.HasDefaultCpuRequestM()},
		DefaultCpuLimitM:       pgtype.Int4{Int32: req.Msg.GetDefaultCpuLimitM(), Valid: req.Msg.HasDefaultCpuLimitM()},
	}

	if _, err := s.queries.ProjectLimitsUpsert(ctx, params); err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == pgerrcode.CheckViolation {
			switch pgErr.ConstraintName {
			case dbconst.ConstraintProjectLimitsCkMemoryLimitGteRequest:
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("memory limit must be greater than or equal to memory request"))
			case dbconst.ConstraintProjectLimitsCkCpuLimitGteRequest:
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("CPU limit must be greater than or equal to CPU request"))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update project limits: %w", err))
	}

	s.logger.InfoContext(ctx, "project limits updated", "project_id", projectID)

	return connect.NewResponse(organizationv1.UpdateProjectLimitsResponse_builder{}.Build()), nil
}
