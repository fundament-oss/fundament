package fleet

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/common/clusterstatus"
	db "github.com/fundament-oss/fundament/platform-api/pkg/db/gen"
	fleetv1 "github.com/fundament-oss/fundament/platform-api/pkg/proto/gen/fleet/v1"
)

// clusterKey is where a page of clusters ends.
type clusterKey struct {
	Alias string    `json:"a"`
	Name  string    `json:"n"`
	ID    uuid.UUID `json:"i"`
}

// ListClusters reads clusters in batches of the page size and keeps those
// that match the filter, until the page is full or the clusters run out. The
// STALE filter is applied in SQL as well; UNHEALTHY only here, because its
// rule is common/clusterstatus, which has no SQL twin. A full page gets a
// token only once one more match is found, so the last page is never an empty
// one.
func (s *Server) ListClusters(ctx context.Context, req *fleetv1.ListClustersRequest) (*fleetv1.ListClustersResponse, error) {
	hb, err := s.heartbeats(ctx)
	if err != nil {
		return nil, err
	}
	size := pageSize(req.GetPageSize())
	params := db.ClusterListParams{
		StaleBefore: s.staleBefore(hb),
		OnlyStale:   req.GetFilter() == fleetv1.ClusterFilter_CLUSTER_FILTER_STALE,
		PageLimit:   size,
	}
	if req.HasOrganizationId() {
		params.OrganizationID = pgtype.UUID{Bytes: uuid.MustParse(req.GetOrganizationId()), Valid: true}
	}
	var after clusterKey
	ok, err := decodePageToken(req.GetPageToken(), &after)
	if err != nil {
		return nil, err
	}

	resp := fleetv1.ListClustersResponse_builder{}
	var last clusterKey
	for {
		if ok {
			params.AfterAlias = pgtype.Text{String: after.Alias, Valid: true}
			params.AfterName = pgtype.Text{String: after.Name, Valid: true}
			params.AfterID = pgtype.UUID{Bytes: after.ID, Valid: true}
		}
		rows, err := s.queries.ClusterList(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("list clusters: %w", err)
		}
		for i := range rows {
			row := &rows[i]
			if !matchesFilter(row, req.GetFilter()) {
				continue
			}
			if len(resp.Clusters) == int(size) {
				// A match beyond a full page: the next page starts after the
				// last cluster on this one.
				resp.NextPageToken = encodePageToken(last)
				return resp.Build(), nil
			}
			resp.Clusters = append(resp.Clusters, clusterSummaryFrom(row))
			last = clusterKey{Alias: row.OrganizationAlias, Name: row.Name, ID: row.ID}
		}
		if len(rows) < int(size) {
			return resp.Build(), nil
		}
		end := &rows[len(rows)-1]
		after, ok = clusterKey{Alias: end.OrganizationAlias, Name: end.Name, ID: end.ID}, true
	}
}

func matchesFilter(row *db.ClusterListRow, filter fleetv1.ClusterFilter) bool {
	switch filter {
	case fleetv1.ClusterFilter_CLUSTER_FILTER_UNSPECIFIED, fleetv1.ClusterFilter_CLUSTER_FILTER_ALL:
		return true
	case fleetv1.ClusterFilter_CLUSTER_FILTER_UNHEALTHY:
		return unhealthy(row)
	case fleetv1.ClusterFilter_CLUSTER_FILTER_STALE:
		return row.Stale
	default:
		panic(fmt.Sprintf("unhandled cluster filter %v", filter))
	}
}

func (s *Server) GetCluster(ctx context.Context, req *fleetv1.GetClusterRequest) (*fleetv1.GetClusterResponse, error) {
	clusterID := uuid.MustParse(req.GetClusterId())

	hb, err := s.heartbeats(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ClusterList(ctx, db.ClusterListParams{
		StaleBefore: s.staleBefore(hb),
		ClusterID:   pgtype.UUID{Bytes: clusterID, Valid: true},
		PageLimit:   1,
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
	hb, err := s.heartbeats(ctx)
	if err != nil {
		return nil, err
	}
	totals, err := s.queries.FleetTotals(ctx, db.FleetTotalsParams{StaleBefore: s.staleBefore(hb)})
	if err != nil {
		return nil, fmt.Errorf("fleet totals: %w", err)
	}

	// Statuses are derived per cluster, as the console derives them.
	inputs, err := s.queries.ClusterStatusInputList(ctx, db.ClusterStatusInputListParams{})
	if err != nil {
		return nil, fmt.Errorf("list cluster status: %w", err)
	}
	var unhealthyCount, upgrading int32
	for i := range inputs {
		status := clusterstatusOf(&inputs[i])
		if status.NeedsAttention() {
			unhealthyCount++
		}
		if status == clusterstatus.Upgrading {
			upgrading++
		}
	}

	summary := fleetv1.FleetSummary_builder{
		Organizations:             totals.Organizations,
		Clusters:                  totals.Clusters,
		ClustersUnhealthy:         unhealthyCount,
		ClustersUpdating:          upgrading,
		Nodes:                     totals.Nodes,
		NodesReady:                totals.NodesReady,
		Pods:                      totals.Pods,
		PodAllocatable:            totals.PodAllocatable,
		CpuAllocatableM:           totals.CpuAllocatableM,
		MemoryAllocatableMi:       totals.MemoryAllocatableMi,
		DriftFindings:             totals.DriftFindings,
		StaleSnapshots:            totals.StaleSnapshots,
		Reconcile:                 heartbeatToProto(hb.reconcile),
		Inventory:                 heartbeatToProto(hb.inventory),
		SnapshotStaleAfterSeconds: int32(hb.staleAfter().Seconds()),
	}

	return fleetv1.GetFleetSummaryResponse_builder{Summary: summary.Build()}.Build(), nil
}

func heartbeatToProto(row *db.WorkerHeartbeatListRow) *fleetv1.WorkerHeartbeat {
	if row == nil {
		return nil
	}
	return fleetv1.WorkerHeartbeat_builder{
		LastPass:        timestampOrNil(row.LastPass),
		IntervalSeconds: row.IntervalSeconds,
	}.Build()
}
