package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) GetProjectLimits(
	ctx context.Context,
	req *connect.Request[organizationv1.GetProjectLimitsRequest],
) (*connect.Response[organizationv1.GetProjectLimitsResponse], error) {
	projectID := uuid.MustParse(req.Msg.GetProjectId())

	if err := s.checkPermission(ctx, authz.CanView(), authz.Project(projectID)); err != nil {
		return nil, err
	}

	row, err := s.queries.ProjectLimitsGet(ctx, db.ProjectLimitsGetParams{ProjectID: projectID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return connect.NewResponse(organizationv1.GetProjectLimitsResponse_builder{
				Limits: organizationv1.ProjectLimits_builder{}.Build(),
			}.Build()), nil
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get project limits: %w", err))
	}

	return connect.NewResponse(organizationv1.GetProjectLimitsResponse_builder{
		Limits: projectLimitsFromRow(&row),
	}.Build()), nil
}

func projectLimitsFromRow(row *db.ProjectLimitsGetRow) *organizationv1.ProjectLimits {
	limits := organizationv1.ProjectLimits_builder{}.Build()
	if row.DefaultMemoryRequestMi.Valid {
		limits.SetDefaultMemoryRequestMi(row.DefaultMemoryRequestMi.Int32)
	}
	if row.DefaultMemoryLimitMi.Valid {
		limits.SetDefaultMemoryLimitMi(row.DefaultMemoryLimitMi.Int32)
	}
	if row.DefaultCpuRequestM.Valid {
		limits.SetDefaultCpuRequestM(row.DefaultCpuRequestM.Int32)
	}
	if row.DefaultCpuLimitM.Valid {
		limits.SetDefaultCpuLimitM(row.DefaultCpuLimitM.Int32)
	}
	return limits
}
