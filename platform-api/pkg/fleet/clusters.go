package fleet

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	db "github.com/fundament-oss/fundament/platform-api/pkg/db/gen"
	fleetv1 "github.com/fundament-oss/fundament/platform-api/pkg/proto/gen/fleet/v1"
)

func (s *Server) ListClusters(ctx context.Context, req *fleetv1.ListClustersRequest) (*fleetv1.ListClustersResponse, error) {
	params := db.ClusterListParams{StaleBefore: pgtype.Timestamptz{Time: s.staleBefore(), Valid: true}}
	if req.HasOrganizationId() {
		params.OrganizationID = pgtype.UUID{Bytes: uuid.MustParse(req.GetOrganizationId()), Valid: true}
	}

	rows, err := s.queries.ClusterList(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list clusters: %w", err)
	}

	clusters := make([]*fleetv1.ClusterSummary, 0, len(rows))
	for i := range rows {
		if matchesFilter(&rows[i], req.GetFilter()) {
			clusters = append(clusters, clusterSummaryFrom(&rows[i]))
		}
	}

	return fleetv1.ListClustersResponse_builder{Clusters: clusters}.Build(), nil
}

func matchesFilter(row *db.ClusterListRow, filter fleetv1.ClusterFilter) bool {
	switch filter {
	case fleetv1.ClusterFilter_CLUSTER_FILTER_UNSPECIFIED, fleetv1.ClusterFilter_CLUSTER_FILTER_ALL:
		return true
	case fleetv1.ClusterFilter_CLUSTER_FILTER_UNHEALTHY:
		return unhealthy(row)
	case fleetv1.ClusterFilter_CLUSTER_FILTER_STALE:
		return row.Stale
	case fleetv1.ClusterFilter_CLUSTER_FILTER_OVER_LIMIT:
		return overLimit(row)
	default:
		panic(fmt.Sprintf("unhandled cluster filter %v", filter))
	}
}

func (s *Server) GetCluster(ctx context.Context, req *fleetv1.GetClusterRequest) (*fleetv1.GetClusterResponse, error) {
	clusterID := uuid.MustParse(req.GetClusterId())

	rows, err := s.queries.ClusterList(ctx, db.ClusterListParams{
		StaleBefore: pgtype.Timestamptz{Time: s.staleBefore(), Valid: true},
		ClusterID:   pgtype.UUID{Bytes: clusterID, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("get cluster: %w", err)
	}
	if len(rows) == 0 {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("cluster not found"))
	}
	row := &rows[0]

	pools, err := s.queries.NodePoolListByCluster(ctx, db.NodePoolListByClusterParams{ClusterID: clusterID})
	if err != nil {
		return nil, fmt.Errorf("list node pools: %w", err)
	}
	projects, err := s.queries.ProjectListByCluster(ctx, db.ProjectListByClusterParams{ClusterID: clusterID})
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	namespaces, err := s.queries.NamespaceListByCluster(ctx, db.NamespaceListByClusterParams{ClusterID: clusterID})
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}

	resp := fleetv1.GetClusterResponse_builder{
		Cluster: clusterSummaryFrom(row),
		Limits:  limitsFrom(row.MaxNodesPerCluster, row.MaxNodePoolsPerCluster, row.MaxNodesPerNodePool),
	}

	shoot, err := s.queries.GardenerShootGetByCluster(ctx, db.GardenerShootGetByClusterParams{ClusterID: clusterID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return nil, fmt.Errorf("get shoot: %w", err)
	default:
		resp.Shoot = fleetv1.GardenerShoot_builder{
			Name:              shoot.ShootName,
			Namespace:         shoot.ShootNamespace,
			KubernetesVersion: shoot.KubernetesVersion,
			Region:            shoot.Region,
			Created:           timestampOrNil(shoot.ShootCreated),
			LastSeen:          timestampOrNil(shoot.LastSeen),
		}.Build()
	}

	for i := range pools {
		p := &pools[i]
		resp.NodePools = append(resp.NodePools, fleetv1.NodePool_builder{
			Id:           p.ID.String(),
			Name:         p.Name,
			MachineType:  p.MachineType,
			AutoscaleMin: p.AutoscaleMin,
			AutoscaleMax: p.AutoscaleMax,
			Nodes:        p.Nodes.Int32,
			NodesReady:   p.NodesReady.Int32,
			Observed:     timestampOrNil(p.Observed),
		}.Build())
	}

	byProject := make(map[uuid.UUID][]*fleetv1.Namespace, len(projects))
	for i := range namespaces {
		ns := &namespaces[i]
		byProject[ns.ProjectID] = append(byProject[ns.ProjectID], fleetv1.Namespace_builder{
			Id:       ns.ID.String(),
			Name:     ns.Name,
			Pods:     ns.Pods.Int32,
			Observed: timestampOrNil(ns.Observed),
		}.Build())
	}
	for i := range projects {
		p := &projects[i]
		resp.Projects = append(resp.Projects, fleetv1.Project_builder{
			Id:         p.ID.String(),
			Name:       p.Name,
			Alias:      p.Alias,
			Namespaces: byProject[p.ID],
		}.Build())
	}

	return resp.Build(), nil
}

func (s *Server) GetFleetSummary(ctx context.Context, _ *fleetv1.GetFleetSummaryRequest) (*fleetv1.GetFleetSummaryResponse, error) {
	totals, err := s.queries.FleetTotals(ctx)
	if err != nil {
		return nil, fmt.Errorf("fleet totals: %w", err)
	}

	findings, err := s.driftFindings(ctx)
	if err != nil {
		return nil, err
	}
	var stale int32
	for _, f := range findings {
		if f.GetKind() == fleetv1.DriftKind_DRIFT_KIND_SNAPSHOT_STALE {
			stale++
		}
	}

	summary := fleetv1.FleetSummary_builder{
		Organizations:       totals.Organizations,
		Clusters:            totals.Clusters,
		ClustersUnhealthy:   totals.ClustersUnhealthy,
		ClustersUpdating:    totals.ClustersUpdating,
		Nodes:               totals.Nodes,
		NodesReady:          totals.NodesReady,
		Pods:                totals.Pods,
		PodAllocatable:      totals.PodAllocatable,
		CpuAllocatableM:     totals.CpuAllocatableM,
		MemoryAllocatableMi: totals.MemoryAllocatableMi,
		DriftFindings:       int32(len(findings)), //nolint:gosec // one finding per cluster or shoot
		StaleSnapshots:      stale,
		StaleAfterSeconds:   int32(s.staleAfter.Seconds()),
	}
	if totals.GardenerInventoryRefreshed.Valid {
		summary.GardenerInventoryRefreshed = timestamppb.New(totals.GardenerInventoryRefreshed.Time)
	}

	return fleetv1.GetFleetSummaryResponse_builder{Summary: summary.Build()}.Build(), nil
}
