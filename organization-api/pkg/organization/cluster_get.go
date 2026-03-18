package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) GetClusterByName(
	ctx context.Context,
	req *connect.Request[organizationv1.GetClusterByNameRequest],
) (*connect.Response[organizationv1.GetClusterResponse], error) {
	cluster, err := s.queries.ClusterGetByName(ctx, db.ClusterGetByNameParams{
		Name: req.Msg.GetName(),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
	}

	if err := s.checkPermission(ctx, authz.CanViewCluster(), authz.Organization(cluster.OrganizationID)); err != nil {
		return nil, err
	}

	return connect.NewResponse(organizationv1.GetClusterResponse_builder{
		Cluster: clusterDetailsFromRow(&db.ClusterGetByIDRow{
			ID:                 cluster.ID,
			OrganizationID:     cluster.OrganizationID,
			Name:               cluster.Name,
			Region:             cluster.Region,
			KubernetesVersion:  cluster.KubernetesVersion,
			Created:            cluster.Created,
			Deleted:            cluster.Deleted,
			Synced:             cluster.Synced,
			SyncError:          cluster.SyncError,
			SyncAttempts:       cluster.SyncAttempts,
			ShootStatus:        cluster.ShootStatus,
			ShootStatusMessage: cluster.ShootStatusMessage,
			ShootStatusUpdated: cluster.ShootStatusUpdated,
		}),
	}.Build()), nil
}

func (s *Server) GetCluster(
	ctx context.Context,
	req *connect.Request[organizationv1.GetClusterRequest],
) (*connect.Response[organizationv1.GetClusterResponse], error) {
	clusterID := uuid.MustParse(req.Msg.GetClusterId())

	cluster, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{
		ID: clusterID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
	}

	if err := s.checkPermission(ctx, authz.CanViewCluster(), authz.Organization(cluster.OrganizationID)); err != nil {
		return nil, err
	}

	return connect.NewResponse(organizationv1.GetClusterResponse_builder{
		Cluster: clusterDetailsFromRow(&cluster),
	}.Build()), nil
}

func (s *Server) GetClusterActivity(
	ctx context.Context,
	req *connect.Request[organizationv1.GetClusterActivityRequest],
) (*connect.Response[organizationv1.GetClusterActivityResponse], error) {
	clusterID := uuid.MustParse(req.Msg.GetClusterId())

	cluster, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{
		ID: clusterID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
	}

	if err := s.checkPermission(ctx, authz.CanViewCluster(), authz.Organization(cluster.OrganizationID)); err != nil {
		return nil, err
	}

	limit := req.Msg.GetLimit()
	if limit <= 0 {
		limit = 50
	}

	events, err := s.queries.ClusterGetEvents(ctx, db.ClusterGetEventsParams{
		ClusterID: clusterID,
		Limit:     limit,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster events: %w", err))
	}

	return connect.NewResponse(organizationv1.GetClusterActivityResponse_builder{
		Events: clusterEventsFromRows(events),
	}.Build()), nil
}

func (s *Server) GetKubeconfig(
	ctx context.Context,
	req *connect.Request[organizationv1.GetKubeconfigRequest],
) (*connect.Response[organizationv1.GetKubeconfigResponse], error) {
	clusterID := uuid.MustParse(req.Msg.GetClusterId())

	cluster, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{
		ID: clusterID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
	}

	if err := s.checkPermission(ctx, authz.CanViewCluster(), authz.Organization(cluster.OrganizationID)); err != nil {
		return nil, err
	}

	kubeconfig := buildKubeconfig(&cluster)

	return connect.NewResponse(organizationv1.GetKubeconfigResponse_builder{
		KubeconfigContent: kubeconfig,
	}.Build()), nil
}

func clusterDetailsFromRow(row *db.ClusterGetByIDRow) *organizationv1.ClusterDetails {
	return organizationv1.ClusterDetails_builder{
		Id:                row.ID.String(),
		Name:              row.Name,
		Region:            row.Region,
		KubernetesVersion: row.KubernetesVersion,
		Status:            clusterStatusFromDB(row.Deleted, row.ShootStatus),
		Created:           timestamppb.New(row.Created.Time),
		ResourceUsage:     nil, // Stub
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

func buildKubeconfig(cluster *db.ClusterGetByIDRow) string {
	return fmt.Sprintf(`apiVersion: v1
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
}
