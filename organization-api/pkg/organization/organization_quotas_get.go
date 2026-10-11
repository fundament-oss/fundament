package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// GetOrganizationQuotas shows the current organization's quotas next to what
// counts against them. Read-only: quotas are set with funops by an operator.
func (s *Server) GetOrganizationQuotas(
	ctx context.Context,
	_ *organizationv1.GetOrganizationQuotasRequest,
) (*organizationv1.GetOrganizationQuotasResponse, error) {
	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	if err := s.checkPermission(ctx, authz.CanView(), authz.Organization(organizationID)); err != nil {
		return nil, err
	}

	clusters, err := s.queries.OrganizationQuotaClustersGet(ctx, db.OrganizationQuotaClustersGetParams{ID: organizationID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("organization not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to read cluster quota: %w", err))
	}

	rows, err := s.queries.OrganizationMachineQuotaList(ctx, db.OrganizationMachineQuotaListParams{OrganizationID: organizationID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list node quotas: %w", err))
	}

	nodes := make([]*organizationv1.NodeQuota, len(rows))
	for i := range rows {
		nodes[i] = organizationv1.NodeQuota_builder{
			Region:      rows[i].Region,
			MachineType: rows[i].MachineType,
			Quota:       rows[i].MaxNodes,
			InUse:       clampInt32(rows[i].NodesInUse),
		}.Build()
	}

	return organizationv1.GetOrganizationQuotasResponse_builder{
		Clusters: organizationv1.ClusterQuota_builder{
			Quota: clusters.QuotaClusters,
			InUse: clampInt32(clusters.ClustersInUse),
		}.Build(),
		Nodes: nodes,
	}.Build(), nil
}

// clampInt32 narrows a count to the proto's int32; a sum of int32 maxima can
// in theory exceed it, and then the ceiling says enough.
func clampInt32(n int64) int32 {
	const maxInt32 = int64(^uint32(0) >> 1)
	if n > maxInt32 {
		return int32(maxInt32)
	}
	return int32(n)
}
