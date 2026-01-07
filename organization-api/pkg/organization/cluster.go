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
	"github.com/fundament-oss/fundament/organization-api/pkg/organization/adapter"
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

	queries := db.New(WithTenant(s.db.Pool, claims.TenantID))
	clusters, err := queries.ClusterListByTenantID(ctx, claims.TenantID)
	if err != nil {
		s.logger.Error("failed to list clusters", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list clusters: %w", err))
	}

	return connect.NewResponse(&organizationv1.ListClustersResponse{
		Clusters: adapter.FromClustersSummary(clusters),
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

	var cluster db.OrganizationCluster

	if err := WithTxTenant(ctx, s.db.Pool, claims.TenantID, func(q *db.Queries) error {
		cluster, err = q.ClusterGetByID(ctx, input.ClusterID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
			}

			s.logger.Error("failed to get cluster", "error", err)
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return connect.NewResponse(&organizationv1.GetClusterResponse{
		Cluster: adapter.FromClusterDetail(cluster),
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

	input := adapter.ToClusterCreate(req.Msg)
	if err := s.validator.Validate(input); err != nil {
		return nil, err
	}

	var cluster db.OrganizationCluster
	if err := WithTxTenant(ctx, s.db.Pool, claims.TenantID, func(q *db.Queries) error {
		params := db.ClusterCreateParams{
			ID:                uuid.New(),
			TenantID:          claims.TenantID,
			Name:              req.Msg.Name,
			Region:            req.Msg.Region,
			KubernetesVersion: req.Msg.KubernetesVersion,
			Status:            "unspecified",
		}

		cluster, err = q.ClusterCreate(ctx, params)
		if err != nil {
			s.logger.Error("failed to create cluster", "error", err)
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create cluster: %w", err))
		}

		return nil
	}); err != nil {
		return nil, err
	}

	s.logger.Info("cluster created",
		"cluster_id", cluster.ID,
		"tenant_id", claims.TenantID,
		"name", cluster.Name,
		"region", cluster.Region,
	)

	return connect.NewResponse(&organizationv1.CreateClusterResponse{
		ClusterId: cluster.ID.String(),
		Status:    adapter.FromClusterStatus(cluster.Status),
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

	input, err := adapter.ToClusterUpdate(req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err := s.validator.Validate(input); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	var cluster db.OrganizationCluster
	if err := WithTxTenant(ctx, s.db.Pool, claims.TenantID, func(q *db.Queries) error {
		params := db.ClusterUpdateParams{
			ID: input.ClusterID,
		}

		if req.Msg.KubernetesVersion != nil {
			params.KubernetesVersion = pgtype.Text{String: *req.Msg.KubernetesVersion, Valid: true}
		}

		cluster, err = q.ClusterUpdate(ctx, params)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
			}

			s.logger.Error("failed to update cluster", "error", err)
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update cluster: %w", err))
		}

		return nil
	}); err != nil {
		return nil, err
	}

	s.logger.Info("cluster updated", "cluster_id", cluster.ID, "name", cluster.Name)

	return connect.NewResponse(&organizationv1.UpdateClusterResponse{
		Cluster: adapter.FromClusterDetail(cluster),
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

	if err := WithTxTenant(ctx, s.db.Pool, claims.TenantID, func(q *db.Queries) error {
		if err := q.ClusterDelete(ctx, clusterID); err != nil {
			s.logger.Error("failed to delete cluster", "error", err)
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete cluster: %w", err))
		}

		return nil
	}); err != nil {
		return nil, err
	}

	s.logger.Info("cluster deleted", "cluster_id", clusterID)

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

	if err := WithTxTenant(ctx, s.db.Pool, claims.TenantID, func(q *db.Queries) error {
		// Verify cluster exists and belongs to tenant
		_, err = q.ClusterGetByID(ctx, input.ClusterID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
			}
			s.logger.Error("failed to get cluster", "error", err)
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
		}

		return nil
	}); err != nil {
		return nil, err
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

	var cluster db.OrganizationCluster

	if err := WithTxTenant(ctx, s.db.Pool, claims.TenantID, func(q *db.Queries) error {
		cluster, err = q.ClusterGetByID(ctx, input.ClusterID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
			}

			s.logger.Error("failed to get cluster", "error", err)
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
		}

		return nil
	}); err != nil {
		return nil, err
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
