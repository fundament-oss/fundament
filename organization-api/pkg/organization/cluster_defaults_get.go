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
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) GetClusterDefaults(
	ctx context.Context,
	req *organizationv1.GetClusterDefaultsRequest,
) (*organizationv1.GetClusterDefaultsResponse, error) {
	clusterID := uuid.MustParse(req.GetClusterId())

	if err := s.checkPermission(ctx, authz.CanView(), authz.Cluster(clusterID)); err != nil {
		return nil, err
	}

	row, err := s.queries.ClusterDefaultsGet(ctx, db.ClusterDefaultsGetParams{ID: clusterID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster defaults: %w", err))
	}

	return organizationv1.GetClusterDefaultsResponse_builder{
		Defaults: containerDefaults(
			row.DefaultMemoryRequestMi,
			row.DefaultMemoryLimitMi,
			row.DefaultCpuRequestM,
			row.DefaultCpuLimitM,
		),
		Suggested: suggestedContainerDefaults(),
	}.Build(), nil
}

// containerDefaults builds the proto message from four nullable columns: a NULL
// column leaves its field absent, which is how "no default is set" travels.
func containerDefaults(memoryRequestMi, memoryLimitMi, cpuRequestM, cpuLimitM pgtype.Int4) *organizationv1.ContainerDefaults {
	defaults := organizationv1.ContainerDefaults_builder{}.Build()
	if memoryRequestMi.Valid {
		defaults.SetMemoryRequestMi(memoryRequestMi.Int32)
	}
	if memoryLimitMi.Valid {
		defaults.SetMemoryLimitMi(memoryLimitMi.Int32)
	}
	if cpuRequestM.Valid {
		defaults.SetCpuRequestM(cpuRequestM.Int32)
	}
	if cpuLimitM.Valid {
		defaults.SetCpuLimitM(cpuLimitM.Int32)
	}
	return defaults
}

// int4 carries a present proto field through as a non-NULL column, an absent
// one as NULL.
func int4(value int32, present bool) pgtype.Int4 {
	return pgtype.Int4{Int32: value, Valid: present}
}
