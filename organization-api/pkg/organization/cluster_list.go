package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) ListClusters(
	ctx context.Context,
	req *connect.Request[organizationv1.ListClustersRequest],
) (*connect.Response[organizationv1.ListClustersResponse], error) {
	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	clusters, err := s.queries.ClusterListByOrganizationID(ctx, db.ClusterListByOrganizationIDParams{
		OrganizationID: organizationID,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list clusters: %w", err))
	}

	summaries := make([]*organizationv1.ClusterSummary, 0, len(clusters))
	for i := range clusters {
		summaries = append(summaries, clusterSummaryFromListRow(&clusters[i]))
	}

	return connect.NewResponse(&organizationv1.ListClustersResponse{
		Clusters: summaries,
	}), nil
}

func clusterSummaryFromListRow(row *db.TenantCluster) *organizationv1.ClusterSummary {
	return &organizationv1.ClusterSummary{
		Id:     row.ID.String(),
		Name:   row.Name,
		Status: clusterStatusFromDB(row.Status),
		Region: row.Region,
	}
}
