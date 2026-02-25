package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"github.com/fundament-oss/fundament/common/authz"
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

	if err := s.checkPermission(ctx, authz.CanListClusters(), authz.Organization(organizationID)); err != nil {
		return nil, err
	}

	clusters, err := s.queries.ClusterList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list clusters: %w", err))
	}

	summaries := make([]*organizationv1.ListClustersResponse_ClusterSummary, 0, len(clusters))
	for i := range clusters {
		summaries = append(summaries, clusterSummaryFromListRow(&clusters[i]))
	}

	return connect.NewResponse(organizationv1.ListClustersResponse_builder{
		Clusters: summaries,
	}.Build()), nil
}

func clusterSummaryFromListRow(row *db.ClusterListRow) *organizationv1.ListClustersResponse_ClusterSummary {
	return organizationv1.ListClustersResponse_ClusterSummary_builder{
		Id:            row.ID.String(),
		Name:          row.Name,
		Status:        clusterStatusFromDB(row.Deleted, row.ShootStatus),
		Region:        row.Region,
		ProjectCount:  0, // Stub
		NodePoolCount: 0, // Stub
		SyncState: syncStateFromRow(
			row.Synced,
			row.SyncError,
			row.SyncAttempts,
			row.ShootStatus,
			row.ShootStatusMessage,
			row.ShootStatusUpdated,
		),
	}.Build()
}
