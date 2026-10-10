package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) UpdateProjectDefaults(
	ctx context.Context,
	req *organizationv1.UpdateProjectDefaultsRequest,
) (*organizationv1.UpdateProjectDefaultsResponse, error) {
	projectID := uuid.MustParse(req.GetProjectId())

	if err := s.checkPermission(ctx, authz.CanEdit(), authz.Project(projectID)); err != nil {
		return nil, err
	}

	// The whole set is replaced: an absent field inherits the cluster's value,
	// so a missing defaults message inherits everything.
	defaults := req.GetDefaults()
	params := db.ProjectDefaultsUpdateParams{
		ID:                     projectID,
		DefaultMemoryRequestMi: int4(defaults.GetMemoryRequestMi(), defaults.HasMemoryRequestMi()),
		DefaultMemoryLimitMi:   int4(defaults.GetMemoryLimitMi(), defaults.HasMemoryLimitMi()),
		DefaultCpuRequestM:     int4(defaults.GetCpuRequestM(), defaults.HasCpuRequestM()),
		DefaultCpuLimitM:       int4(defaults.GetCpuLimitM(), defaults.HasCpuLimitM()),
	}

	rows, err := s.queries.ProjectDefaultsUpdate(ctx, params)
	if err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
			switch {
			case pgErr.Code == pgerrcode.CheckViolation:
				switch pgErr.ConstraintName {
				case dbconst.ConstraintProjectsCkDefaultMemoryLimitGteRequest:
					return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("memory limit must be greater than or equal to memory request"))
				case dbconst.ConstraintProjectsCkDefaultCpuLimitGteRequest:
					return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("CPU limit must be greater than or equal to CPU request"))
				}
			// The trigger says which value is too high and what the cluster
			// allows, and its text is written to be shown to the user.
			case pgErr.Code == pgerrcode.RaiseException && pgErr.Hint == dbconst.HintProjectDefaultsExceedCluster:
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New(pgErr.Message))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update project defaults: %w", err))
	}
	if rows == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project not found"))
	}

	s.logger.InfoContext(ctx, "project defaults updated", "project_id", projectID)

	return organizationv1.UpdateProjectDefaultsResponse_builder{}.Build(), nil
}
