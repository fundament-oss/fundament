package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *OrganizationServer) ListClusters(
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
	for _, c := range clusters {
		summaries = append(summaries, &organizationv1.ClusterSummary{
			Id:     c.ID.String(),
			Name:   c.Name,
			Status: clusterStatusFromDB(c.Status),
			Region: c.Region,
		})
	}

	return connect.NewResponse(&organizationv1.ListClustersResponse{
		Clusters: summaries,
	}), nil
}

func (s *OrganizationServer) GetCluster(
	ctx context.Context,
	req *connect.Request[organizationv1.GetClusterRequest],
) (*connect.Response[organizationv1.GetClusterResponse], error) {
	clusterID := uuid.MustParse(req.Msg.ClusterId)

	cluster, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{
		ID: clusterID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
	}

	return connect.NewResponse(&organizationv1.GetClusterResponse{
		Cluster: &organizationv1.ClusterDetails{
			Id:                cluster.ID.String(),
			Name:              cluster.Name,
			Region:            cluster.Region,
			KubernetesVersion: cluster.KubernetesVersion,
			Status:            clusterStatusFromDB(cluster.Status),
			CreatedAt:         timestamppb.New(cluster.Created.Time),
			ResourceUsage:     nil, // Stub
		},
	}), nil
}

func (s *OrganizationServer) GetClusterByName(
	ctx context.Context,
	req *connect.Request[organizationv1.GetClusterByNameRequest],
) (*connect.Response[organizationv1.GetClusterResponse], error) {
	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	cluster, err := s.queries.ClusterGetByName(ctx, db.ClusterGetByNameParams{
		OrganizationID: organizationID,
		Name:           req.Msg.Name,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
	}

	return connect.NewResponse(&organizationv1.GetClusterResponse{
		Cluster: &organizationv1.ClusterDetails{
			Id:                cluster.ID.String(),
			Name:              cluster.Name,
			Region:            cluster.Region,
			KubernetesVersion: cluster.KubernetesVersion,
			Status:            clusterStatusFromDB(cluster.Status),
			CreatedAt:         timestamppb.New(cluster.Created.Time),
			ResourceUsage:     nil, // Stub
		},
	}), nil
}

func (s *OrganizationServer) CreateCluster(
	ctx context.Context,
	req *connect.Request[organizationv1.CreateClusterRequest],
) (*connect.Response[organizationv1.CreateClusterResponse], error) {
	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	params := db.ClusterCreateParams{
		OrganizationID:    organizationID,
		Name:              req.Msg.Name,
		Region:            req.Msg.Region,
		KubernetesVersion: req.Msg.KubernetesVersion,
		Status:            dbconst.ClusterStatus_Unspecified,
	}

	clusterID, err := s.queries.ClusterCreate(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create cluster: %w", err))
	}

	s.logger.InfoContext(ctx, "cluster created",
		"cluster_id", clusterID,
		"organization_id", organizationID,
		"name", req.Msg.Name,
		"region", req.Msg.Region,
	)

	return connect.NewResponse(&organizationv1.CreateClusterResponse{
		ClusterId: clusterID.String(),
	}), nil
}

func (s *OrganizationServer) UpdateCluster(
	ctx context.Context,
	req *connect.Request[organizationv1.UpdateClusterRequest],
) (*connect.Response[emptypb.Empty], error) {
	clusterID := uuid.MustParse(req.Msg.ClusterId)

	params := db.ClusterUpdateParams{
		ID: clusterID,
	}

	if req.Msg.KubernetesVersion != nil {
		params.KubernetesVersion = pgtype.Text{String: *req.Msg.KubernetesVersion, Valid: true}
	}

	rowsAffected, err := s.queries.ClusterUpdate(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update cluster: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
	}

	s.logger.InfoContext(ctx, "cluster updated", "cluster_id", clusterID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (s *OrganizationServer) DeleteCluster(
	ctx context.Context,
	req *connect.Request[organizationv1.DeleteClusterRequest],
) (*connect.Response[emptypb.Empty], error) {
	clusterID := uuid.MustParse(req.Msg.ClusterId)

	rowsAffected, err := s.queries.ClusterDelete(ctx, db.ClusterDeleteParams{ID: clusterID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete cluster: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
	}

	s.logger.InfoContext(ctx, "cluster deleted", "cluster_id", clusterID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (s *OrganizationServer) GetClusterActivity(
	ctx context.Context,
	req *connect.Request[organizationv1.GetClusterActivityRequest],
) (*connect.Response[organizationv1.GetClusterActivityResponse], error) {
	clusterID := uuid.MustParse(req.Msg.ClusterId)

	// Verify cluster exists and belongs to tenant
	_, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{
		ID: clusterID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
	}

	// Stub: return empty activities
	return connect.NewResponse(&organizationv1.GetClusterActivityResponse{
		Activities: []*organizationv1.ActivityEntry{},
	}), nil
}

func (s *OrganizationServer) GetKubeconfig(
	ctx context.Context,
	req *connect.Request[organizationv1.GetKubeconfigRequest],
) (*connect.Response[organizationv1.GetKubeconfigResponse], error) {
	clusterID := uuid.MustParse(req.Msg.ClusterId)

	cluster, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{
		ID: clusterID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
	}

	// Stub: return placeholder kubeconfig
	kubeconfig := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://%s.organizationv1.io:6443
  name: %s
contexts:
- context:
    cluster: %s
    user: %s
  name: %s
current-context: %s
users:
- name: %s
  user: {}
`, cluster.ID.String(), cluster.Name, cluster.Name, cluster.Name, cluster.Name, cluster.Name, cluster.Name)

	return connect.NewResponse(&organizationv1.GetKubeconfigResponse{
		KubeconfigContent: kubeconfig,
	}), nil
}

func clusterStatusFromDB(status dbconst.ClusterStatus) organizationv1.ClusterStatus {
	switch status {
	case dbconst.ClusterStatus_Provisioning:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING
	case dbconst.ClusterStatus_Starting:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_STARTING
	case dbconst.ClusterStatus_Running:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING
	case dbconst.ClusterStatus_Upgrading:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_UPGRADING
	case dbconst.ClusterStatus_Error:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR
	case dbconst.ClusterStatus_Stopping:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPING
	case dbconst.ClusterStatus_Stopped:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPED
	case dbconst.ClusterStatus_Unspecified:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_UNSPECIFIED
	default:
		panic(fmt.Sprintf("unknown cluster status from db: %s", status))
	}
}
