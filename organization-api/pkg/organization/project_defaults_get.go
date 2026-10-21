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

func (s *Server) GetProjectDefaults(
	ctx context.Context,
	req *organizationv1.GetProjectDefaultsRequest,
) (*organizationv1.GetProjectDefaultsResponse, error) {
	projectID := uuid.MustParse(req.GetProjectId())

	if err := s.checkPermission(ctx, authz.CanView(), authz.Project(projectID)); err != nil {
		return nil, err
	}

	row, err := s.queries.ProjectDefaultsGet(ctx, db.ProjectDefaultsGetParams{ID: projectID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get project defaults: %w", err))
	}

	// The cluster's values go to anyone who may view the project: the console
	// needs them to show what an unset field inherits and how far a custom
	// value may go.
	return organizationv1.GetProjectDefaultsResponse_builder{
		Defaults: containerDefaults(
			row.DefaultMemoryRequestMi,
			row.DefaultMemoryLimitMi,
			row.DefaultCpuRequestM,
			row.DefaultCpuLimitM,
		),
		ClusterDefaults: containerDefaults(
			row.ClusterDefaultMemoryRequestMi,
			row.ClusterDefaultMemoryLimitMi,
			row.ClusterDefaultCpuRequestM,
			row.ClusterDefaultCpuLimitM,
		),
	}.Build(), nil
}
