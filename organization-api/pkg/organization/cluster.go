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
	"github.com/fundament-oss/fundament/organization-api/pkg/organization/adapter"
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

	return connect.NewResponse(&organizationv1.ListClustersResponse{
		Clusters: adapter.FromClustersSummary(clusters),
	}), nil
}

func (s *OrganizationServer) GetCluster(
	ctx context.Context,
	req *connect.Request[organizationv1.GetClusterRequest],
) (*connect.Response[organizationv1.GetClusterResponse], error) {
	clusterID, err := uuid.Parse(req.Msg.ClusterId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid cluster id: %w", err))
	}

	cluster, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{
		ID: clusterID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
	}

	nodePools, err := s.queries.NodePoolListByClusterID(ctx, db.NodePoolListByClusterIDParams{ClusterID: clusterID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get node pools: %w", err))
	}

	clusterDetails := adapter.FromClusterDetail(cluster)
	clusterDetails.NodePools = adapter.FromNodePools(nodePools)

	return connect.NewResponse(&organizationv1.GetClusterResponse{
		Cluster: clusterDetails,
	}), nil
}

func (s *OrganizationServer) CreateCluster(
	ctx context.Context,
	req *connect.Request[organizationv1.CreateClusterRequest],
) (*connect.Response[organizationv1.CreateClusterResponse], error) {
	input := adapter.ToClusterCreate(req.Msg)
	if err := s.validator.Validate(input); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("pool begin"))
	}

	defer tx.Rollback(ctx)

	params := db.ClusterCreateParams{
		OrganizationID:    organizationID,
		Name:              input.Name,
		Region:            input.Region,
		KubernetesVersion: input.KubernetesVersion,
		Status:            "unspecified",
	}

	txQueries := s.queries.WithTx(tx)

	cluster, err := txQueries.ClusterCreate(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create cluster: %w", err))
	}

	// Create node pools
	for _, spec := range req.Msg.NodePools {
		params := db.NodePoolCreateParams{
			ClusterID:    cluster.ID,
			Name:         spec.Name,
			MachineType:  spec.MachineType,
			AutoscaleMin: spec.AutoscaleMin,
			AutoscaleMax: spec.AutoscaleMax,
		}

		if _, err := txQueries.NodePoolCreate(ctx, params); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create node pool %q: %w", spec.Name, err))
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit transaction: %w", err))
	}

	s.logger.InfoContext(ctx, "cluster created",
		"cluster_id", cluster.ID,
		"organization_id", organizationID,
		"name", cluster.Name,
		"region", cluster.Region,
		"node_pool_count", len(req.Msg.NodePools),
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
	input, err := adapter.ToClusterUpdate(req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err := s.validator.Validate(input); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	params := db.ClusterUpdateParams{
		ID: input.ClusterID,
	}

	if input.KubernetesVersion != nil {
		params.KubernetesVersion = pgtype.Text{String: *input.KubernetesVersion, Valid: true}
	}

	cluster, err := s.queries.ClusterUpdate(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update cluster: %w", err))
	}

	nodePools, err := s.queries.NodePoolListByClusterID(ctx, db.NodePoolListByClusterIDParams{ClusterID: input.ClusterID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get node pools: %w", err))
	}

	s.logger.InfoContext(ctx, "cluster updated", "cluster_id", cluster.ID, "name", cluster.Name)

	clusterDetails := adapter.FromClusterDetail(cluster)
	clusterDetails.NodePools = adapter.FromNodePools(nodePools)

	return connect.NewResponse(&organizationv1.UpdateClusterResponse{
		Cluster: clusterDetails,
	}), nil
}

func (s *OrganizationServer) DeleteCluster(
	ctx context.Context,
	req *connect.Request[organizationv1.DeleteClusterRequest],
) (*connect.Response[organizationv1.DeleteClusterResponse], error) {
	clusterID, err := uuid.Parse(req.Msg.ClusterId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid cluster id: %w", err))
	}

	err = s.queries.ClusterDelete(ctx, db.ClusterDeleteParams{
		ID: clusterID,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete cluster: %w", err))
	}

	s.logger.InfoContext(ctx, "cluster deleted", "cluster_id", clusterID)

	return connect.NewResponse(&organizationv1.DeleteClusterResponse{
		Success: true,
	}), nil
}

func (s *OrganizationServer) GetClusterActivity(
	ctx context.Context,
	req *connect.Request[organizationv1.GetClusterActivityRequest],
) (*connect.Response[organizationv1.GetClusterActivityResponse], error) {
	clusterID, err := uuid.Parse(req.Msg.ClusterId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid cluster id: %w", err))
	}

	// Verify cluster exists and belongs to tenant
	_, err = s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{
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
	clusterID, err := uuid.Parse(req.Msg.ClusterId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid cluster id: %w", err))
	}

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
