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

// shootStatusReady mirrors cluster-worker's gardener.StatusReady value
// written to cluster.shoot_status in the DB.
const shootStatusReady = "ready"

func (s *Server) GetClusterByName(
	ctx context.Context,
	req *organizationv1.GetClusterByNameRequest,
) (*organizationv1.GetClusterResponse, error) {
	cluster, err := s.queries.ClusterGetByName(ctx, db.ClusterGetByNameParams{
		Name: req.GetName(),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
	}

	// Auth is done after the DB call because we dont know the cluster ID yet.
	// This does leave us open for enumerate attackes because attackers can distinguise between not found and permission denied.
	// We could always return cluster not found instead of permission errors.
	if err := s.checkPermission(ctx, authz.CanView(), authz.Cluster(cluster.ID)); err != nil {
		return nil, err
	}

	row := &db.ClusterGetByIDRow{
		ID:                 cluster.ID,
		OrganizationID:     cluster.OrganizationID,
		Name:               cluster.Name,
		Region:             cluster.Region,
		KubernetesVersion:  cluster.KubernetesVersion,
		Created:            cluster.Created,
		Deleted:            cluster.Deleted,
		ShootStatus:        cluster.ShootStatus,
		ShootStatusMessage: cluster.ShootStatusMessage,
		ShootStatusUpdated: cluster.ShootStatusUpdated,
		OutboxStatus:       cluster.OutboxStatus,
		OutboxRetries:      cluster.OutboxRetries,
		OutboxError:        cluster.OutboxError,
	}
	details := clusterDetailsFromRow(row)

	return organizationv1.GetClusterResponse_builder{
		Cluster: details,
	}.Build(), nil
}

func (s *Server) GetCluster(
	ctx context.Context,
	req *organizationv1.GetClusterRequest,
) (*organizationv1.GetClusterResponse, error) {
	clusterID := uuid.MustParse(req.GetClusterId())

	if err := s.checkPermission(ctx, authz.CanView(), authz.Cluster(clusterID)); err != nil {
		return nil, err
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

	if cluster.Deleted.Valid {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
	}

	details := clusterDetailsFromRow(&cluster)

	return organizationv1.GetClusterResponse_builder{
		Cluster: details,
	}.Build(), nil
}

func (s *Server) GetClusterActivity(
	ctx context.Context,
	req *organizationv1.GetClusterActivityRequest,
) (*organizationv1.GetClusterActivityResponse, error) {
	clusterID := uuid.MustParse(req.GetClusterId())

	if err := s.checkPermission(ctx, authz.CanView(), authz.Cluster(clusterID)); err != nil {
		return nil, err
	}

	_, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{
		ID: clusterID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
	}

	limit := req.GetLimit()
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

	return organizationv1.GetClusterActivityResponse_builder{
		Events: clusterEventsFromRows(events),
	}.Build(), nil
}

func (s *Server) GetKubeconfig(
	ctx context.Context,
	req *organizationv1.GetKubeconfigRequest,
) (*organizationv1.GetKubeconfigResponse, error) {
	clusterID := uuid.MustParse(req.GetClusterId())

	if err := s.checkPermission(ctx, authz.CanView(), authz.Cluster(clusterID)); err != nil {
		return nil, err
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

	if !cluster.ShootStatus.Valid || cluster.ShootStatus.String != shootStatusReady {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("cluster not ready yet"))
	}

	if s.config.KubeAPIProxyURL == "" {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("kube-api-proxy URL not configured"))
	}
	proxyURL := s.config.KubeAPIProxyURL + "/clusters/" + clusterID.String()

	kubeconfig := buildKubeconfig(clusterID.String(), proxyURL)

	return organizationv1.GetKubeconfigResponse_builder{
		KubeconfigContent: kubeconfig,
	}.Build(), nil
}

func clusterDetailsFromRow(row *db.ClusterGetByIDRow) *organizationv1.ClusterDetails {
	builder := organizationv1.ClusterDetails_builder{
		Id:                row.ID.String(),
		Name:              row.Name,
		Region:            row.Region,
		KubernetesVersion: row.KubernetesVersion,
		Status:            clusterStatusFromDB(row.Deleted, row.ShootStatus),
		Created:           timestamppb.New(row.Created.Time),
		ResourceUsage:     nil, // Stub
		SyncState: syncStateFromRow(
			row.OutboxStatus,
			row.OutboxRetries,
			row.OutboxError,
			row.ShootStatus,
			row.ShootStatusMessage,
			row.ShootStatusUpdated,
		),
	}
	return builder.Build()
}

func buildKubeconfig(clusterID, serverURL string) string {
	clusterName := "fundament-" + clusterID
	userName := "fundament-user-" + clusterID

	return fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: %s
  name: %s
contexts:
- context:
    cluster: %s
    user: %s
  name: %s
current-context: %s
users:
- name: %s
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1
      command: functl
      args:
      - cluster
      - token
      - %s
      interactiveMode: Never
      provideClusterInfo: false
`, serverURL, clusterName, clusterName, userName, clusterName, clusterName, userName, clusterID)
}
