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

func (s *Server) GetOrganizationLimits(
	ctx context.Context,
	req *connect.Request[organizationv1.GetOrganizationLimitsRequest],
) (*connect.Response[organizationv1.GetOrganizationLimitsResponse], error) {
	organizationID := uuid.MustParse(req.Msg.GetId())

	if err := s.checkPermission(ctx, authz.CanView(), authz.Organization(organizationID)); err != nil {
		return nil, err
	}

	row, err := s.queries.OrganizationLimitsGet(ctx, db.OrganizationLimitsGetParams{OrganizationID: organizationID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return connect.NewResponse(organizationv1.GetOrganizationLimitsResponse_builder{
				Limits: organizationv1.OrganizationLimits_builder{}.Build(),
			}.Build()), nil
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get organization limits: %w", err))
	}

	return connect.NewResponse(organizationv1.GetOrganizationLimitsResponse_builder{
		Limits: organizationLimitsFromRow(&row),
	}.Build()), nil
}

func organizationLimitsFromRow(row *db.OrganizationLimitsGetRow) *organizationv1.OrganizationLimits {
	limits := organizationv1.OrganizationLimits_builder{}.Build()
	if row.MaxNodesPerCluster.Valid {
		limits.SetMaxNodesPerCluster(row.MaxNodesPerCluster.Int32)
	}
	if row.MaxNodePoolsPerCluster.Valid {
		limits.SetMaxNodePoolsPerCluster(row.MaxNodePoolsPerCluster.Int32)
	}
	if row.MaxNodesPerNodePool.Valid {
		limits.SetMaxNodesPerNodePool(row.MaxNodesPerNodePool.Int32)
	}
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
