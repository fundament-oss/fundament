package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/models"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *OrganizationServer) ListClusters(
	ctx context.Context,
	req *connect.Request[organizationv1.ListClustersRequest],
) (*connect.Response[organizationv1.ListClustersResponse], error) {
	claims, err := s.validateRequest(req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	queries := s.tenantQueries(claims.TenantID)

	clusters, err := queries.ClusterListByTenantID(ctx, claims.TenantID)
	if err != nil {
		s.logger.Error("failed to list clusters", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list clusters: %w", err))
	}

	summaries := make([]*organizationv1.ClusterSummary, 0, len(clusters))
	for _, c := range clusters {
		summaries = append(summaries, &organizationv1.ClusterSummary{
			Id:            c.ID.String(),
			Name:          c.Name,
			Status:        dbStatusToProto(c.Status),
			Region:        c.Region,
			ProjectCount:  0, // Stub
			NodePoolCount: 0, // Stub
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
	claims, err := s.validateRequest(req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	clusterID, err := uuid.Parse(req.Msg.ClusterId)
	if err != nil {
		return nil, fmt.Errorf("cluster id parse: %w", err)
	}

	input := models.ClusterGet{ClusterID: clusterID}
	if err := s.validator.Validate(input); err != nil {
		return nil, err
	}

	queries := s.tenantQueries(claims.TenantID)

	// RLS ensures we can only see clusters belonging to our tenant
	cluster, err := queries.ClusterGetByID(ctx, input.ClusterID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		s.logger.Error("failed to get cluster", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
	}

	return connect.NewResponse(&organizationv1.GetClusterResponse{
		Cluster: dbClusterToProtoDetails(cluster),
	}), nil
}

func (s *OrganizationServer) CreateCluster(
	ctx context.Context,
	req *connect.Request[organizationv1.CreateClusterRequest],
) (*connect.Response[organizationv1.CreateClusterResponse], error) {
	claims, err := s.validateRequest(req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	input := models.ClusterCreate{
		Name:              req.Msg.Name,
		Region:            req.Msg.Region,
		KubernetesVersion: req.Msg.KubernetesVersion,
	}
	if err := s.validator.Validate(input); err != nil {
		return nil, err
	}

	queries := s.tenantQueries(claims.TenantID)

	params := db.ClusterCreateParams{
		ID:                uuid.New(),
		TenantID:          claims.TenantID,
		Name:              req.Msg.Name,
		Region:            req.Msg.Region,
		KubernetesVersion: req.Msg.KubernetesVersion,
		Status:            db.OrganizationClusterStatusUnspecified,
	}

	cluster, err := queries.ClusterCreate(ctx, params)
	if err != nil {
		s.logger.Error("failed to create cluster", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create cluster: %w", err))
	}

	s.logger.Info("cluster created",
		"cluster_id", cluster.ID,
		"tenant_id", claims.TenantID,
		"name", cluster.Name,
		"region", cluster.Region,
	)

	return connect.NewResponse(&organizationv1.CreateClusterResponse{
		ClusterId: cluster.ID.String(),
		Status:    dbStatusToProto(cluster.Status),
	}), nil
}

func (s *OrganizationServer) UpdateCluster(
	ctx context.Context,
	req *connect.Request[organizationv1.UpdateClusterRequest],
) (*connect.Response[organizationv1.UpdateClusterResponse], error) {
	claims, err := s.validateRequest(req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	clusterID, err := uuid.Parse(req.Msg.ClusterId)
	if err != nil {
		return nil, fmt.Errorf("cluster id parse: %w", err)
	}

	input := models.ClusterUpdate{ClusterID: clusterID}
	if err := s.validator.Validate(input); err != nil {
		return nil, err
	}

	queries := s.tenantQueries(claims.TenantID)

	params := db.ClusterUpdateParams{
		ID: input.ClusterID,
	}

	if req.Msg.KubernetesVersion != nil {
		params.KubernetesVersion = pgtype.Text{String: *req.Msg.KubernetesVersion, Valid: true}
	}

	// RLS ensures we can only update clusters belonging to our tenant
	cluster, err := queries.ClusterUpdate(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		s.logger.Error("failed to update cluster", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update cluster: %w", err))
	}

	s.logger.Info("cluster updated", "cluster_id", cluster.ID, "name", cluster.Name)

	return connect.NewResponse(&organizationv1.UpdateClusterResponse{
		Cluster: dbClusterToProtoDetails(cluster),
	}), nil
}

func (s *OrganizationServer) DeleteCluster(
	ctx context.Context,
	req *connect.Request[organizationv1.DeleteClusterRequest],
) (*connect.Response[organizationv1.DeleteClusterResponse], error) {
	claims, err := s.validateRequest(req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	clusterID, err := uuid.Parse(req.Msg.ClusterId)
	if err != nil {
		return nil, fmt.Errorf("cluster id parse: %w", err)
	}

	input := models.ClusterDelete{ClusterID: clusterID}
	if err := s.validator.Validate(input); err != nil {
		return nil, err
	}

	queries := s.tenantQueries(claims.TenantID)

	// RLS ensures we can only delete clusters belonging to our tenant
	err = queries.ClusterDelete(ctx, input.ClusterID)
	if err != nil {
		s.logger.Error("failed to delete cluster", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete cluster: %w", err))
	}

	s.logger.Info("cluster deleted", "cluster_id", input.ClusterID)

	return connect.NewResponse(&organizationv1.DeleteClusterResponse{
		Success: true,
	}), nil
}

func (s *OrganizationServer) GetClusterActivity(
	ctx context.Context,
	req *connect.Request[organizationv1.GetClusterActivityRequest],
) (*connect.Response[organizationv1.GetClusterActivityResponse], error) {
	claims, err := s.validateRequest(req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	clusterID, err := uuid.Parse(req.Msg.ClusterId)
	if err != nil {
		return nil, fmt.Errorf("cluster id parse: %w", err)
	}

	input := models.ClusterGetActivity{ClusterID: clusterID}
	if err := s.validator.Validate(input); err != nil {
		return nil, err
	}

	queries := s.tenantQueries(claims.TenantID)

	// Verify cluster exists and belongs to tenant (via RLS)
	_, err = queries.ClusterGetByID(ctx, input.ClusterID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		s.logger.Error("failed to get cluster", "error", err)
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
	claims, err := s.validateRequest(req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	clusterID, err := uuid.Parse(req.Msg.ClusterId)
	if err != nil {
		return nil, fmt.Errorf("cluster id parse: %w", err)
	}

	input := models.ClusterGetKubeconfig{ClusterID: clusterID}
	if err := s.validator.Validate(input); err != nil {
		return nil, err
	}

	queries := s.tenantQueries(claims.TenantID)

	// RLS ensures we can only see clusters belonging to our tenant
	cluster, err := queries.ClusterGetByID(ctx, input.ClusterID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		s.logger.Error("failed to get cluster", "error", err)
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

// Helper functions for cluster

func dbStatusToProto(status db.OrganizationClusterStatus) organizationv1.ClusterStatus {
	switch status {
	case db.OrganizationClusterStatusProvisioning:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING
	case db.OrganizationClusterStatusStarting:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_STARTING
	case db.OrganizationClusterStatusRunning:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING
	case db.OrganizationClusterStatusUpgrading:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_UPGRADING
	case db.OrganizationClusterStatusError:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR
	case db.OrganizationClusterStatusStopping:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPING
	case db.OrganizationClusterStatusStopped:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPED
	default:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_UNSPECIFIED
	}
}

func dbClusterToProtoDetails(c db.OrganizationCluster) *organizationv1.ClusterDetails {
	return &organizationv1.ClusterDetails{
		Id:                c.ID.String(),
		Name:              c.Name,
		Region:            c.Region,
		KubernetesVersion: c.KubernetesVersion,
		Status:            dbStatusToProto(c.Status),
		CreatedAt: &organizationv1.Timestamp{
			Value: c.Created.Time.Format("2006-01-02T15:04:05Z07:00"),
		},
		ResourceUsage: nil, // Stub
		NodePools:     nil, // Stub
		Members:       nil, // Stub
		Projects:      nil, // Stub
	}
}
