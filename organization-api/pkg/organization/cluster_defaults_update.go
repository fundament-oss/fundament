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

func (s *Server) UpdateClusterDefaults(
	ctx context.Context,
	req *organizationv1.UpdateClusterDefaultsRequest,
) (*organizationv1.UpdateClusterDefaultsResponse, error) {
	clusterID := uuid.MustParse(req.GetClusterId())

	if err := s.checkPermission(ctx, authz.CanEdit(), authz.Cluster(clusterID)); err != nil {
		return nil, err
	}

	// The whole set is replaced: an absent field clears that default, so a
	// missing defaults message clears all four.
	defaults := req.GetDefaults()
	params := db.ClusterDefaultsUpdateParams{
		ID:                     clusterID,
		DefaultMemoryRequestMi: int4(defaults.GetMemoryRequestMi(), defaults.HasMemoryRequestMi()),
		DefaultMemoryLimitMi:   int4(defaults.GetMemoryLimitMi(), defaults.HasMemoryLimitMi()),
		DefaultCpuRequestM:     int4(defaults.GetCpuRequestM(), defaults.HasCpuRequestM()),
		DefaultCpuLimitM:       int4(defaults.GetCpuLimitM(), defaults.HasCpuLimitM()),
	}

	rows, err := s.queries.ClusterDefaultsUpdate(ctx, params)
	if err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
			switch {
			case pgErr.Code == pgerrcode.CheckViolation:
				switch pgErr.ConstraintName {
				case dbconst.ConstraintClustersCkDefaultMemoryLimitGteRequest:
					return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("memory limit must be greater than or equal to memory request"))
				case dbconst.ConstraintClustersCkDefaultCpuLimitGteRequest:
					return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("CPU limit must be greater than or equal to CPU request"))
				}
			// The trigger names the project to lower first, and its text is
			// written to be shown to the admin.
			case pgErr.Code == pgerrcode.RaiseException && pgErr.Hint == dbconst.HintClusterDefaultsBelowProject:
				return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New(pgErr.Message))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update cluster defaults: %w", err))
	}
	if rows == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
	}

	s.logger.InfoContext(ctx, "cluster defaults updated", "cluster_id", clusterID)

	return organizationv1.UpdateClusterDefaultsResponse_builder{}.Build(), nil
}
