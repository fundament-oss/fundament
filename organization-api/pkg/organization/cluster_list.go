package organization

import (
	"context"
	"fmt"
	"math"

	"connectrpc.com/connect"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) ListClusters(
	ctx context.Context,
	req *organizationv1.ListClustersRequest,
) (*organizationv1.ListClustersResponse, error) {
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

	return organizationv1.ListClustersResponse_builder{
		Clusters: summaries,
	}.Build(), nil
}

func clusterSummaryFromListRow(row *db.ClusterListRow) *organizationv1.ListClustersResponse_ClusterSummary {
	return organizationv1.ListClustersResponse_ClusterSummary_builder{
		Id:            row.ID.String(),
		Name:          row.Name,
		Status:        clusterStatusFromDB(row.Deleted, row.ShootStatus),
		Region:        row.Region,
		ProjectCount:  countAsInt32(row.ProjectCount),
		NodePoolCount: countAsInt32(row.NodePoolCount),
		SyncState: syncStateFromRow(
			row.OutboxStatus,
			row.OutboxRetries,
			row.OutboxError,
			row.ShootStatus,
			row.ShootStatusMessage,
			row.ShootStatusUpdated,
		),
	}.Build()
}

// COUNT(*) comes back as int64 while the wire format is int32. A cluster does
// not hold two billion projects or node pools, but the clamp keeps the
// conversion from ever wrapping into a negative count.
func countAsInt32(count int64) int32 {
	if count < 0 {
		return 0
	}
	if count > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(count)
}
