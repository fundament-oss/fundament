package organization

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	prom "github.com/fundament-oss/fundament/organization-api/pkg/prometheus"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

const (
	bytesPerGiB = 1073741824.0
	bytesPerMB  = 1_000_000.0
)

// promClient returns the appropriate Prometheus client based on the server's configured
// prometheusURL: empty or "mock" → MockClient (if configured) or StubClient,
// otherwise HTTPClient targeting the given URL.
func (s *Server) promClient() prom.Client {
	switch s.prometheusURL {
	case "", "mock":
		if s.mockPromClient != nil {
			return s.mockPromClient
		}
		return prom.StubClient{}
	default:
		return prom.NewHTTPClient(s.prometheusURL)
	}
}

// -- Cluster-level RPCs --

func (s *Server) GetClusterWorkloadMetrics(
	ctx context.Context,
	req *connect.Request[organizationv1.GetClusterWorkloadMetricsRequest],
) (*connect.Response[organizationv1.GetClusterWorkloadMetricsResponse], error) {
	clusterID := uuid.MustParse(req.Msg.GetClusterId())

	if err := s.checkPermission(ctx, authz.CanView(), authz.Cluster(clusterID)); err != nil {
		return nil, err
	}

	if _, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{ID: clusterID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get cluster: %w", err))
	}

	client := s.promClient()
	now := time.Now()

	var (
		cpuUsed, cpuTotal   float64
		memUsed, memTotal   float64
		podsUsed, podsTotal float64

		nodeCPUUsed, nodeCPUTotal   []prom.Sample
		nodeMemUsed, nodeMemTotal   []prom.Sample
		nodePodsUsed, nodePodsTotal []prom.Sample

		nsCPU, nsMem, nsPods []prom.Sample
		nsCPUReq, nsCPULim   []prom.Sample
		nsMemReq, nsMemLim   []prom.Sample
		nsNetRx, nsNetTx     []prom.Sample
	)

	g, gctx := errgroup.WithContext(ctx)

	qi := func(dst *float64, label, query string) {
		g.Go(func() error {
			v, err := querySingleValue(gctx, client, query, now)
			if err != nil {
				return fmt.Errorf("%s: %w", label, err)
			}
			*dst = v
			return nil
		})
	}
	qs := func(dst *[]prom.Sample, label, query string) {
		g.Go(func() error {
			samples, err := client.Query(gctx, query, now)
			if err != nil {
				return fmt.Errorf("%s: %w", label, err)
			}
			*dst = samples
			return nil
		})
	}

	qi(&cpuUsed, "query cpu used", `sum(rate(container_cpu_usage_seconds_total{container!=""}[5m]))`)
	qi(&cpuTotal, "query cpu total", `sum(kube_node_status_capacity{resource="cpu"})`)
	qi(&memUsed, "query mem used", `sum(container_memory_working_set_bytes{container!=""})`)
	qi(&memTotal, "query mem total", `sum(kube_node_status_capacity{resource="memory"})`)
	qi(&podsUsed, "query pods used", `count(kube_pod_info)`)
	qi(&podsTotal, "query pods total", `sum(kube_node_status_capacity{resource="pods"})`)

	qs(&nodeCPUUsed, "query per-node cpu used", `sum(rate(container_cpu_usage_seconds_total{container!=""}[5m])) by (node)`)
	qs(&nodeCPUTotal, "query per-node cpu total", `sum(kube_node_status_capacity{resource="cpu"}) by (node)`)
	qs(&nodeMemUsed, "query per-node mem used", `sum(container_memory_working_set_bytes{container!=""}) by (node)`)
	qs(&nodeMemTotal, "query per-node mem total", `sum(kube_node_status_capacity{resource="memory"}) by (node)`)
	qs(&nodePodsUsed, "query per-node pods used", `count(kube_pod_info) by (node)`)
	qs(&nodePodsTotal, "query per-node pods total", `sum(kube_node_status_capacity{resource="pods"}) by (node)`)

	qs(&nsCPU, "query per-namespace cpu", `sum(rate(container_cpu_usage_seconds_total{container!=""}[5m])) by (namespace)`)
	qs(&nsMem, "query per-namespace mem", `sum(container_memory_working_set_bytes{container!=""}) by (namespace)`)
	qs(&nsPods, "query per-namespace pods", `count(kube_pod_info) by (namespace)`)
	qs(&nsCPUReq, "query per-namespace cpu requests", `sum(kube_pod_container_resource_requests{resource="cpu"}) by (namespace)`)
	qs(&nsCPULim, "query per-namespace cpu limits", `sum(kube_pod_container_resource_limits{resource="cpu"}) by (namespace)`)
	qs(&nsMemReq, "query per-namespace mem requests", `sum(kube_pod_container_resource_requests{resource="memory"}) by (namespace)`)
	qs(&nsMemLim, "query per-namespace mem limits", `sum(kube_pod_container_resource_limits{resource="memory"}) by (namespace)`)
	qs(&nsNetRx, "query per-namespace net rx", `sum(rate(container_network_receive_bytes_total[5m])) by (namespace)`)
	qs(&nsNetTx, "query per-namespace net tx", `sum(rate(container_network_transmit_bytes_total[5m])) by (namespace)`)

	if err := g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	totals := organizationv1.ResourceUsageInfo_builder{
		Cpu:    makeResourceUsage(cpuUsed, cpuTotal, "cores"),
		Memory: makeResourceUsage(memUsed/bytesPerGiB, memTotal/bytesPerGiB, "GiB"),
		Pods:   makeResourceUsage(podsUsed, podsTotal, "pods"),
	}.Build()

	return connect.NewResponse(organizationv1.GetClusterWorkloadMetricsResponse_builder{
		Totals:     totals,
		Nodes:      buildNodeMetrics(nodeCPUUsed, nodeCPUTotal, nodeMemUsed, nodeMemTotal, nodePodsUsed, nodePodsTotal),
		Namespaces: buildNamespaceMetrics(nsCPU, nsMem, nsPods, nsCPUReq, nsCPULim, nsMemReq, nsMemLim, nsNetRx, nsNetTx),
	}.Build()), nil
}

func (s *Server) GetClusterWorkloadTimeSeries(
	ctx context.Context,
	req *connect.Request[organizationv1.GetClusterWorkloadTimeSeriesRequest],
) (*connect.Response[organizationv1.GetWorkloadTimeSeriesResponse], error) {
	clusterID := uuid.MustParse(req.Msg.GetClusterId())

	if err := s.checkPermission(ctx, authz.CanView(), authz.Cluster(clusterID)); err != nil {
		return nil, err
	}

	if _, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{ID: clusterID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get cluster: %w", err))
	}

	client := s.promClient()
	start, end, step := resolveTimeRange(req.Msg.HasStart(), req.Msg.GetStart().AsTime(), req.Msg.HasEnd(), req.Msg.GetEnd().AsTime(), req.Msg.GetStepSeconds())

	var (
		cpuSeries, memSeries, podSeries []prom.TimeSeries
		netRxSeries, netTxSeries        []prom.TimeSeries
	)

	g, gctx := errgroup.WithContext(ctx)

	qr := func(dst *[]prom.TimeSeries, label, query string) {
		g.Go(func() error {
			ts, err := client.QueryRange(gctx, query, start, end, step)
			if err != nil {
				return fmt.Errorf("%s: %w", label, err)
			}
			*dst = ts
			return nil
		})
	}

	qr(&cpuSeries, "query cpu time-series", `sum(rate(container_cpu_usage_seconds_total{container!=""}[5m]))`)
	qr(&memSeries, "query mem time-series", `sum(container_memory_working_set_bytes{container!=""})`)
	qr(&podSeries, "query pod time-series", `count(kube_pod_info)`)
	qr(&netRxSeries, "query net rx time-series", `sum(rate(container_network_receive_bytes_total[5m]))`)
	qr(&netTxSeries, "query net tx time-series", `sum(rate(container_network_transmit_bytes_total[5m]))`)

	if err := g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(organizationv1.GetWorkloadTimeSeriesResponse_builder{
		CpuCores:           timeSeriesFirstToProto(cpuSeries, 1),
		MemoryGib:          timeSeriesFirstToProto(memSeries, 1.0/bytesPerGiB),
		PodCount:           timeSeriesFirstToProto(podSeries, 1),
		NetworkReceiveMbS:  timeSeriesFirstToProto(netRxSeries, 1.0/bytesPerMB),
		NetworkTransmitMbS: timeSeriesFirstToProto(netTxSeries, 1.0/bytesPerMB),
	}.Build()), nil
}

// -- Org-level RPCs --

func (s *Server) GetOrgWorkloadMetrics(
	ctx context.Context,
	_ *connect.Request[organizationv1.GetOrgWorkloadMetricsRequest],
) (*connect.Response[organizationv1.GetOrgWorkloadMetricsResponse], error) {
	clusters, err := s.queries.ClusterList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list clusters: %w", err))
	}

	now := time.Now()

	type clusterResult struct {
		id                   string
		name                 string
		cpuUsed, cpuTotal    float64
		memUsed, memTotal    float64
		podsUsed, podsTotal  float64
		nsCPU, nsMem, nsPods []prom.Sample
		nsCPUReq, nsCPULim   []prom.Sample
		nsMemReq, nsMemLim   []prom.Sample
		nsNetRx, nsNetTx     []prom.Sample
	}

	results := make([]clusterResult, len(clusters))
	g, gctx := errgroup.WithContext(ctx)
	// Limit outer concurrency to avoid overwhelming Prometheus and connection pools
	// when an org has many clusters (each cluster fans out to ~15 sub-queries).
	g.SetLimit(10)

	for i, cl := range clusters {
		i, cl := i, cl
		g.Go(func() error {
			client := s.promClient()
			r := &results[i]
			r.id = cl.ID.String()
			r.name = cl.Name

			sub, subCtx := errgroup.WithContext(gctx)

			qi := func(dst *float64, label, query string) {
				sub.Go(func() error {
					v, err := querySingleValue(subCtx, client, query, now)
					if err != nil {
						return fmt.Errorf("%s [%s]: %w", label, cl.Name, err)
					}
					*dst = v
					return nil
				})
			}
			qs := func(dst *[]prom.Sample, label, query string) {
				sub.Go(func() error {
					samples, err := client.Query(subCtx, query, now)
					if err != nil {
						return fmt.Errorf("%s [%s]: %w", label, cl.Name, err)
					}
					*dst = samples
					return nil
				})
			}

			qi(&r.cpuUsed, "query cpu used", `sum(rate(container_cpu_usage_seconds_total{container!=""}[5m]))`)
			qi(&r.cpuTotal, "query cpu total", `sum(kube_node_status_capacity{resource="cpu"})`)
			qi(&r.memUsed, "query mem used", `sum(container_memory_working_set_bytes{container!=""})`)
			qi(&r.memTotal, "query mem total", `sum(kube_node_status_capacity{resource="memory"})`)
			qi(&r.podsUsed, "query pods used", `count(kube_pod_info)`)
			qi(&r.podsTotal, "query pods total", `sum(kube_node_status_capacity{resource="pods"})`)

			qs(&r.nsCPU, "query per-namespace cpu", `sum(rate(container_cpu_usage_seconds_total{container!=""}[5m])) by (namespace)`)
			qs(&r.nsMem, "query per-namespace mem", `sum(container_memory_working_set_bytes{container!=""}) by (namespace)`)
			qs(&r.nsPods, "query per-namespace pods", `count(kube_pod_info) by (namespace)`)
			qs(&r.nsCPUReq, "query per-namespace cpu requests", `sum(kube_pod_container_resource_requests{resource="cpu"}) by (namespace)`)
			qs(&r.nsCPULim, "query per-namespace cpu limits", `sum(kube_pod_container_resource_limits{resource="cpu"}) by (namespace)`)
			qs(&r.nsMemReq, "query per-namespace mem requests", `sum(kube_pod_container_resource_requests{resource="memory"}) by (namespace)`)
			qs(&r.nsMemLim, "query per-namespace mem limits", `sum(kube_pod_container_resource_limits{resource="memory"}) by (namespace)`)
			qs(&r.nsNetRx, "query per-namespace net rx", `sum(rate(container_network_receive_bytes_total[5m])) by (namespace)`)
			qs(&r.nsNetTx, "query per-namespace net tx", `sum(rate(container_network_transmit_bytes_total[5m])) by (namespace)`)

			return sub.Wait()
		})
	}

	if err := g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Aggregate across clusters.
	var cpuUsed, cpuTotal, memUsed, memTotal, podsUsed, podsTotal float64
	clusterSummaries := make([]*organizationv1.ClusterWorkloadSummary, 0, len(results))
	allNsCPU := make([][]prom.Sample, len(results))
	allNsMem := make([][]prom.Sample, len(results))
	allNsPods := make([][]prom.Sample, len(results))
	allNsCPUReq := make([][]prom.Sample, len(results))
	allNsCPULim := make([][]prom.Sample, len(results))
	allNsMemReq := make([][]prom.Sample, len(results))
	allNsMemLim := make([][]prom.Sample, len(results))
	allNsNetRx := make([][]prom.Sample, len(results))
	allNsNetTx := make([][]prom.Sample, len(results))

	for i, r := range results {
		cpuUsed += r.cpuUsed
		cpuTotal += r.cpuTotal
		memUsed += r.memUsed
		memTotal += r.memTotal
		podsUsed += r.podsUsed
		podsTotal += r.podsTotal

		clusterSummaries = append(clusterSummaries, organizationv1.ClusterWorkloadSummary_builder{
			ClusterId:   r.id,
			ClusterName: r.name,
			Cpu:         makeResourceUsage(r.cpuUsed, r.cpuTotal, "cores"),
			Memory:      makeResourceUsage(r.memUsed/bytesPerGiB, r.memTotal/bytesPerGiB, "GiB"),
			Pods:        makeResourceUsage(r.podsUsed, r.podsTotal, "pods"),
		}.Build())

		allNsCPU[i] = r.nsCPU
		allNsMem[i] = r.nsMem
		allNsPods[i] = r.nsPods
		allNsCPUReq[i] = r.nsCPUReq
		allNsCPULim[i] = r.nsCPULim
		allNsMemReq[i] = r.nsMemReq
		allNsMemLim[i] = r.nsMemLim
		allNsNetRx[i] = r.nsNetRx
		allNsNetTx[i] = r.nsNetTx
	}

	totals := organizationv1.ResourceUsageInfo_builder{
		Cpu:    makeResourceUsage(cpuUsed, cpuTotal, "cores"),
		Memory: makeResourceUsage(memUsed/bytesPerGiB, memTotal/bytesPerGiB, "GiB"),
		Pods:   makeResourceUsage(podsUsed, podsTotal, "pods"),
	}.Build()

	nsCPU := mergeSamples(allNsCPU, "namespace")
	nsMem := mergeSamples(allNsMem, "namespace")
	nsPods := mergeSamples(allNsPods, "namespace")
	nsCPUReq := mergeSamples(allNsCPUReq, "namespace")
	nsCPULim := mergeSamples(allNsCPULim, "namespace")
	nsMemReq := mergeSamples(allNsMemReq, "namespace")
	nsMemLim := mergeSamples(allNsMemLim, "namespace")
	nsNetRx := mergeSamples(allNsNetRx, "namespace")
	nsNetTx := mergeSamples(allNsNetTx, "namespace")

	return connect.NewResponse(organizationv1.GetOrgWorkloadMetricsResponse_builder{
		Totals:     totals,
		Clusters:   clusterSummaries,
		Namespaces: buildNamespaceMetrics(nsCPU, nsMem, nsPods, nsCPUReq, nsCPULim, nsMemReq, nsMemLim, nsNetRx, nsNetTx),
	}.Build()), nil
}

func (s *Server) GetOrgWorkloadTimeSeries(
	ctx context.Context,
	req *connect.Request[organizationv1.GetOrgWorkloadTimeSeriesRequest],
) (*connect.Response[organizationv1.GetWorkloadTimeSeriesResponse], error) {
	clusters, err := s.queries.ClusterList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list clusters: %w", err))
	}

	start, end, step := resolveTimeRange(req.Msg.HasStart(), req.Msg.GetStart().AsTime(), req.Msg.HasEnd(), req.Msg.GetEnd().AsTime(), req.Msg.GetStepSeconds())

	type clusterTSResult struct {
		cpu, mem, pods, netRx, netTx []prom.TimeSeries
	}

	results := make([]clusterTSResult, len(clusters))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(10)

	for i, cl := range clusters {
		i, cl := i, cl
		g.Go(func() error {
			client := s.promClient()
			r := &results[i]

			sub, subCtx := errgroup.WithContext(gctx)

			qr := func(dst *[]prom.TimeSeries, label, query string) {
				sub.Go(func() error {
					ts, err := client.QueryRange(subCtx, query, start, end, step)
					if err != nil {
						return fmt.Errorf("%s [%s]: %w", label, cl.Name, err)
					}
					*dst = ts
					return nil
				})
			}

			qr(&r.cpu, "query cpu time-series", `sum(rate(container_cpu_usage_seconds_total{container!=""}[5m]))`)
			qr(&r.mem, "query mem time-series", `sum(container_memory_working_set_bytes{container!=""})`)
			qr(&r.pods, "query pod time-series", `count(kube_pod_info)`)
			qr(&r.netRx, "query net rx time-series", `sum(rate(container_network_receive_bytes_total[5m]))`)
			qr(&r.netTx, "query net tx time-series", `sum(rate(container_network_transmit_bytes_total[5m]))`)

			return sub.Wait()
		})
	}

	if err := g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	allCPU := make([][]prom.TimeSeries, len(results))
	allMem := make([][]prom.TimeSeries, len(results))
	allPods := make([][]prom.TimeSeries, len(results))
	allNetRx := make([][]prom.TimeSeries, len(results))
	allNetTx := make([][]prom.TimeSeries, len(results))
	for i, r := range results {
		allCPU[i] = r.cpu
		allMem[i] = r.mem
		allPods[i] = r.pods
		allNetRx[i] = r.netRx
		allNetTx[i] = r.netTx
	}

	return connect.NewResponse(organizationv1.GetWorkloadTimeSeriesResponse_builder{
		CpuCores:           timeSeriesFirstToProto(sumTimeSeries(allCPU), 1),
		MemoryGib:          timeSeriesFirstToProto(sumTimeSeries(allMem), 1.0/bytesPerGiB),
		PodCount:           timeSeriesFirstToProto(sumTimeSeries(allPods), 1),
		NetworkReceiveMbS:  timeSeriesFirstToProto(sumTimeSeries(allNetRx), 1.0/bytesPerMB),
		NetworkTransmitMbS: timeSeriesFirstToProto(sumTimeSeries(allNetTx), 1.0/bytesPerMB),
	}.Build()), nil
}

// -- Project-level RPCs --

func (s *Server) GetProjectWorkloadMetrics(
	ctx context.Context,
	req *connect.Request[organizationv1.GetProjectWorkloadMetricsRequest],
) (*connect.Response[organizationv1.GetProjectWorkloadMetricsResponse], error) {
	projectID := uuid.MustParse(req.Msg.GetProjectId())

	if err := s.checkPermission(ctx, authz.CanView(), authz.Project(projectID)); err != nil {
		return nil, err
	}

	namespaces, err := s.queries.NamespaceListByProjectID(ctx, db.NamespaceListByProjectIDParams{ProjectID: projectID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list project namespaces: %w", err))
	}

	if len(namespaces) == 0 {
		return connect.NewResponse(organizationv1.GetProjectWorkloadMetricsResponse_builder{
			Totals:     organizationv1.ResourceUsageInfo_builder{}.Build(),
			Namespaces: nil,
		}.Build()), nil
	}

	// All namespaces in a project are expected to live on the same cluster.
	// If cross-cluster projects are ever introduced, this must be updated to
	// fan out per-cluster and aggregate results.
	clusterID := namespaces[0].ClusterID
	for _, ns := range namespaces[1:] {
		if ns.ClusterID != clusterID {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("project spans multiple clusters, which is not supported"))
		}
	}

	if _, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{ID: clusterID}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get cluster: %w", err))
	}

	client := s.promClient()
	nsFilter := buildNamespaceFilter(namespaceNames(namespaces))
	now := time.Now()

	var (
		cpuUsed, cpuTotal   float64
		memUsed, memTotal   float64
		podsUsed, podsTotal float64

		nsCPU, nsMem, nsPods []prom.Sample
		nsCPUReq, nsCPULim   []prom.Sample
		nsMemReq, nsMemLim   []prom.Sample
		nsNetRx, nsNetTx     []prom.Sample
	)

	g, gctx := errgroup.WithContext(ctx)

	qi := func(dst *float64, label, query string) {
		g.Go(func() error {
			v, err := querySingleValue(gctx, client, query, now)
			if err != nil {
				return fmt.Errorf("%s: %w", label, err)
			}
			*dst = v
			return nil
		})
	}
	qs := func(dst *[]prom.Sample, label, query string) {
		g.Go(func() error {
			samples, err := client.Query(gctx, query, now)
			if err != nil {
				return fmt.Errorf("%s: %w", label, err)
			}
			*dst = samples
			return nil
		})
	}

	qi(&cpuUsed, "query cpu used", fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{container!="",%s}[5m]))`, nsFilter))
	qi(&cpuTotal, "query cpu total", `sum(kube_node_status_capacity{resource="cpu"})`)
	qi(&memUsed, "query mem used", fmt.Sprintf(`sum(container_memory_working_set_bytes{container!="",%s})`, nsFilter))
	qi(&memTotal, "query mem total", `sum(kube_node_status_capacity{resource="memory"})`)
	qi(&podsUsed, "query pods used", fmt.Sprintf(`count(kube_pod_info{%s})`, nsFilter))
	qi(&podsTotal, "query pods total", `sum(kube_node_status_capacity{resource="pods"})`)

	qs(&nsCPU, "query per-namespace cpu", fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{container!="",%s}[5m])) by (namespace)`, nsFilter))
	qs(&nsMem, "query per-namespace mem", fmt.Sprintf(`sum(container_memory_working_set_bytes{container!="",%s}) by (namespace)`, nsFilter))
	qs(&nsPods, "query per-namespace pods", fmt.Sprintf(`count(kube_pod_info{%s}) by (namespace)`, nsFilter))
	qs(&nsCPUReq, "query per-namespace cpu requests", fmt.Sprintf(`sum(kube_pod_container_resource_requests{resource="cpu",%s}) by (namespace)`, nsFilter))
	qs(&nsCPULim, "query per-namespace cpu limits", fmt.Sprintf(`sum(kube_pod_container_resource_limits{resource="cpu",%s}) by (namespace)`, nsFilter))
	qs(&nsMemReq, "query per-namespace mem requests", fmt.Sprintf(`sum(kube_pod_container_resource_requests{resource="memory",%s}) by (namespace)`, nsFilter))
	qs(&nsMemLim, "query per-namespace mem limits", fmt.Sprintf(`sum(kube_pod_container_resource_limits{resource="memory",%s}) by (namespace)`, nsFilter))
	qs(&nsNetRx, "query per-namespace net rx", fmt.Sprintf(`sum(rate(container_network_receive_bytes_total{%s}[5m])) by (namespace)`, nsFilter))
	qs(&nsNetTx, "query per-namespace net tx", fmt.Sprintf(`sum(rate(container_network_transmit_bytes_total{%s}[5m])) by (namespace)`, nsFilter))

	if err := g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(organizationv1.GetProjectWorkloadMetricsResponse_builder{
		Totals: organizationv1.ResourceUsageInfo_builder{
			Cpu:    makeResourceUsage(cpuUsed, cpuTotal, "cores"),
			Memory: makeResourceUsage(memUsed/bytesPerGiB, memTotal/bytesPerGiB, "GiB"),
			Pods:   makeResourceUsage(podsUsed, podsTotal, "pods"),
		}.Build(),
		Namespaces: buildNamespaceMetrics(nsCPU, nsMem, nsPods, nsCPUReq, nsCPULim, nsMemReq, nsMemLim, nsNetRx, nsNetTx),
	}.Build()), nil
}

func (s *Server) GetProjectWorkloadTimeSeries(
	ctx context.Context,
	req *connect.Request[organizationv1.GetProjectWorkloadTimeSeriesRequest],
) (*connect.Response[organizationv1.GetWorkloadTimeSeriesResponse], error) {
	projectID := uuid.MustParse(req.Msg.GetProjectId())

	if err := s.checkPermission(ctx, authz.CanView(), authz.Project(projectID)); err != nil {
		return nil, err
	}

	namespaces, err := s.queries.NamespaceListByProjectID(ctx, db.NamespaceListByProjectIDParams{ProjectID: projectID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list project namespaces: %w", err))
	}

	if len(namespaces) == 0 {
		return connect.NewResponse(organizationv1.GetWorkloadTimeSeriesResponse_builder{}.Build()), nil
	}

	// All namespaces in a project are expected to live on the same cluster.
	// If cross-cluster projects are ever introduced, this must be updated to
	// fan out per-cluster and aggregate results.
	clusterID := namespaces[0].ClusterID
	for _, ns := range namespaces[1:] {
		if ns.ClusterID != clusterID {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("project spans multiple clusters, which is not supported"))
		}
	}

	if _, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{ID: clusterID}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get cluster: %w", err))
	}

	client := s.promClient()
	nsFilter := buildNamespaceFilter(namespaceNames(namespaces))
	start, end, step := resolveTimeRange(req.Msg.HasStart(), req.Msg.GetStart().AsTime(), req.Msg.HasEnd(), req.Msg.GetEnd().AsTime(), req.Msg.GetStepSeconds())

	var (
		cpuSeries, memSeries, podSeries []prom.TimeSeries
		netRxSeries, netTxSeries        []prom.TimeSeries
	)

	g, gctx := errgroup.WithContext(ctx)

	qr := func(dst *[]prom.TimeSeries, label, query string) {
		g.Go(func() error {
			ts, err := client.QueryRange(gctx, query, start, end, step)
			if err != nil {
				return fmt.Errorf("%s: %w", label, err)
			}
			*dst = ts
			return nil
		})
	}

	qr(&cpuSeries, "query cpu time-series", fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{container!="",%s}[5m]))`, nsFilter))
	qr(&memSeries, "query mem time-series", fmt.Sprintf(`sum(container_memory_working_set_bytes{container!="",%s})`, nsFilter))
	qr(&podSeries, "query pod time-series", fmt.Sprintf(`count(kube_pod_info{%s})`, nsFilter))
	qr(&netRxSeries, "query net rx time-series", fmt.Sprintf(`sum(rate(container_network_receive_bytes_total{%s}[5m]))`, nsFilter))
	qr(&netTxSeries, "query net tx time-series", fmt.Sprintf(`sum(rate(container_network_transmit_bytes_total{%s}[5m]))`, nsFilter))

	if err := g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(organizationv1.GetWorkloadTimeSeriesResponse_builder{
		CpuCores:           timeSeriesFirstToProto(cpuSeries, 1),
		MemoryGib:          timeSeriesFirstToProto(memSeries, 1.0/bytesPerGiB),
		PodCount:           timeSeriesFirstToProto(podSeries, 1),
		NetworkReceiveMbS:  timeSeriesFirstToProto(netRxSeries, 1.0/bytesPerMB),
		NetworkTransmitMbS: timeSeriesFirstToProto(netTxSeries, 1.0/bytesPerMB),
	}.Build()), nil
}

// -- Helper functions --

// querySingleValue executes an instant query and returns the first result value,
// or 0 if no results are returned.
func querySingleValue(ctx context.Context, client prom.Client, query string, t time.Time) (float64, error) {
	samples, err := client.Query(ctx, query, t)
	if err != nil {
		return 0, err
	}
	if len(samples) == 0 {
		return 0, nil
	}
	return samples[0].Value, nil
}

// makeResourceUsage builds a ResourceUsage proto message.
func makeResourceUsage(used, total float64, unit string) *organizationv1.ResourceUsage {
	return organizationv1.ResourceUsage_builder{
		Used:  used,
		Total: total,
		Unit:  unit,
	}.Build()
}

// buildNodeMetrics combines six per-node sample slices into NodeWorkloadMetrics
// messages. Node names are unioned across all slices so that nodes appearing
// in only some results are still included (with zeros for missing values).
func buildNodeMetrics(
	cpuUsed, cpuTotal, memUsed, memTotal, podsUsed, podsTotal []prom.Sample,
) []*organizationv1.NodeWorkloadMetrics {
	type nodeData struct {
		cpuUsed, cpuTotal   float64
		memUsed, memTotal   float64
		podsUsed, podsTotal float64
	}
	nodes := make(map[string]*nodeData)

	ensureNode := func(name string) *nodeData {
		if nodes[name] == nil {
			nodes[name] = &nodeData{}
		}
		return nodes[name]
	}

	for _, s := range cpuUsed {
		ensureNode(s.Labels["node"]).cpuUsed = s.Value
	}
	for _, s := range cpuTotal {
		ensureNode(s.Labels["node"]).cpuTotal = s.Value
	}
	for _, s := range memUsed {
		ensureNode(s.Labels["node"]).memUsed = s.Value
	}
	for _, s := range memTotal {
		ensureNode(s.Labels["node"]).memTotal = s.Value
	}
	for _, s := range podsUsed {
		ensureNode(s.Labels["node"]).podsUsed = s.Value
	}
	for _, s := range podsTotal {
		ensureNode(s.Labels["node"]).podsTotal = s.Value
	}

	result := make([]*organizationv1.NodeWorkloadMetrics, 0, len(nodes))
	for name, d := range nodes {
		result = append(result, organizationv1.NodeWorkloadMetrics_builder{
			Node:   name,
			Cpu:    makeResourceUsage(d.cpuUsed, d.cpuTotal, "cores"),
			Memory: makeResourceUsage(d.memUsed/bytesPerGiB, d.memTotal/bytesPerGiB, "GiB"),
			Pods:   makeResourceUsage(d.podsUsed, d.podsTotal, "pods"),
		}.Build())
	}
	return result
}

// buildNamespaceMetrics combines per-namespace samples into NamespaceWorkloadMetrics
// messages. cpuReq/cpuLim/memReq/memLim come from kube_pod_container_resource_requests/limits;
// netRx/netTx come from container_network_*_bytes_total rates.
func buildNamespaceMetrics(
	cpuSamples, memSamples, podSamples,
	cpuReqSamples, cpuLimSamples, memReqSamples, memLimSamples,
	netRxSamples, netTxSamples []prom.Sample,
) []*organizationv1.NamespaceWorkloadMetrics {
	type nsData struct {
		cpu, mem, pods float64
		cpuReq, cpuLim float64
		memReq, memLim float64
		netRx, netTx   float64
	}
	namespaces := make(map[string]*nsData)

	ensureNS := func(name string) *nsData {
		if namespaces[name] == nil {
			namespaces[name] = &nsData{}
		}
		return namespaces[name]
	}

	for _, s := range cpuSamples {
		ensureNS(s.Labels["namespace"]).cpu = s.Value
	}
	for _, s := range memSamples {
		ensureNS(s.Labels["namespace"]).mem = s.Value
	}
	for _, s := range podSamples {
		ensureNS(s.Labels["namespace"]).pods = s.Value
	}
	for _, s := range cpuReqSamples {
		ensureNS(s.Labels["namespace"]).cpuReq = s.Value
	}
	for _, s := range cpuLimSamples {
		ensureNS(s.Labels["namespace"]).cpuLim = s.Value
	}
	for _, s := range memReqSamples {
		ensureNS(s.Labels["namespace"]).memReq = s.Value
	}
	for _, s := range memLimSamples {
		ensureNS(s.Labels["namespace"]).memLim = s.Value
	}
	for _, s := range netRxSamples {
		ensureNS(s.Labels["namespace"]).netRx = s.Value
	}
	for _, s := range netTxSamples {
		ensureNS(s.Labels["namespace"]).netTx = s.Value
	}

	result := make([]*organizationv1.NamespaceWorkloadMetrics, 0, len(namespaces))
	for name, d := range namespaces {
		result = append(result, organizationv1.NamespaceWorkloadMetrics_builder{
			Namespace:          name,
			CpuCores:           d.cpu,
			MemoryGib:          d.mem / bytesPerGiB,
			Pods:               int32(d.pods),
			CpuRequests:        d.cpuReq,
			CpuLimits:          d.cpuLim,
			MemoryRequestsGib:  d.memReq / bytesPerGiB,
			MemoryLimitsGib:    d.memLim / bytesPerGiB,
			NetworkReceiveMbS:  d.netRx / bytesPerMB,
			NetworkTransmitMbS: d.netTx / bytesPerMB,
		}.Build())
	}
	return result
}

// timeSeriesFirstToProto converts the first TimeSeries result to proto MetricSample
// messages, applying an optional scale factor (e.g. bytes→GiB).
func timeSeriesFirstToProto(series []prom.TimeSeries, scale float64) []*organizationv1.MetricSample {
	if len(series) == 0 {
		return nil
	}
	points := series[0].Samples
	result := make([]*organizationv1.MetricSample, 0, len(points))
	for _, p := range points {
		result = append(result, organizationv1.MetricSample_builder{
			Timestamp: timestamppb.New(p.Time),
			Value:     p.Value * scale,
		}.Build())
	}
	return result
}

// mergeSamples merges per-cluster sample sets by summing values with the same
// label value (e.g. namespace name) across clusters.
func mergeSamples(allSamples [][]prom.Sample, labelKey string) []prom.Sample {
	sums := make(map[string]float64)
	for _, samples := range allSamples {
		for _, s := range samples {
			sums[s.Labels[labelKey]] += s.Value
		}
	}
	result := make([]prom.Sample, 0, len(sums))
	for name, val := range sums {
		result = append(result, prom.Sample{
			Labels: map[string]string{labelKey: name},
			Value:  val,
		})
	}
	return result
}

// sumTimeSeries sums multiple single-TimeSeries results (one per cluster) into
// a single TimeSeries by adding values at matching timestamps.
func sumTimeSeries(allSeries [][]prom.TimeSeries) []prom.TimeSeries {
	sums := make(map[time.Time]float64)
	for _, ts := range allSeries {
		if len(ts) == 0 {
			continue
		}
		for _, dp := range ts[0].Samples {
			sums[dp.Time] += dp.Value
		}
	}
	if len(sums) == 0 {
		return nil
	}
	times := make([]time.Time, 0, len(sums))
	for t := range sums {
		times = append(times, t)
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	points := make([]prom.DataPoint, len(times))
	for i, t := range times {
		points[i] = prom.DataPoint{Time: t, Value: sums[t]}
	}
	return []prom.TimeSeries{{Labels: map[string]string{}, Samples: points}}
}

// promEscapeLabelValue escapes backslashes and double-quotes in a PromQL label value.
func promEscapeLabelValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// buildNamespaceFilter returns a PromQL label selector fragment that matches any
// of the given namespace names: namespace=~"ns1|ns2|ns3".
// names must be non-empty; callers are responsible for guarding against empty slices.
func buildNamespaceFilter(names []string) string {
	if len(names) == 0 {
		// Callers must guard against this. Return a filter that matches nothing:
		// Kubernetes namespace names must be valid DNS labels, so "_" is impossible.
		return `namespace="_"`
	}
	return fmt.Sprintf(`namespace=~"%s"`, strings.Join(names, "|"))
}

// resolveTimeRange returns start, end, and step for a Prometheus range query,
// applying sensible defaults when values are absent or zero.
func resolveTimeRange(hasStart bool, start time.Time, hasEnd bool, end time.Time, stepSeconds int32) (time.Time, time.Time, time.Duration) {
	now := time.Now()
	if !hasEnd {
		end = now
	}
	if !hasStart {
		start = end.Add(-7 * 24 * time.Hour)
	}
	// Default to 5-minute steps as documented in the proto (step_seconds defaults to 300).
	step := 300 * time.Second
	if stepSeconds > 0 {
		step = time.Duration(stepSeconds) * time.Second
	}
	return start, end, step
}

// namespaceNames extracts the name strings from a slice of namespace DB rows.
func namespaceNames(rows []db.NamespaceListByProjectIDRow) []string {
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		names = append(names, r.Name)
	}
	return names
}

// -- Streaming RPCs --

const streamInterval = 15 * time.Second

// StreamOrgWorkloadMetrics streams org-wide metrics every 15 seconds.
func (s *Server) StreamOrgWorkloadMetrics(
	ctx context.Context,
	req *connect.Request[organizationv1.StreamOrgWorkloadMetricsRequest],
	stream *connect.ServerStream[organizationv1.StreamWorkloadMetricsResponse],
) error {
	if err := s.sendOrgSnapshot(ctx, req.Msg, stream); err != nil {
		return err
	}
	ticker := time.NewTicker(streamInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.sendOrgSnapshot(ctx, req.Msg, stream); err != nil {
				return err
			}
		}
	}
}

// StreamClusterWorkloadMetrics streams cluster metrics every 15 seconds.
func (s *Server) StreamClusterWorkloadMetrics(
	ctx context.Context,
	req *connect.Request[organizationv1.StreamClusterWorkloadMetricsRequest],
	stream *connect.ServerStream[organizationv1.StreamWorkloadMetricsResponse],
) error {
	if err := s.sendClusterSnapshot(ctx, req.Msg, stream); err != nil {
		return err
	}
	ticker := time.NewTicker(streamInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.sendClusterSnapshot(ctx, req.Msg, stream); err != nil {
				return err
			}
		}
	}
}

// StreamProjectWorkloadMetrics streams project metrics every 15 seconds.
func (s *Server) StreamProjectWorkloadMetrics(
	ctx context.Context,
	req *connect.Request[organizationv1.StreamProjectWorkloadMetricsRequest],
	stream *connect.ServerStream[organizationv1.StreamWorkloadMetricsResponse],
) error {
	if err := s.sendProjectSnapshot(ctx, req.Msg, stream); err != nil {
		return err
	}
	ticker := time.NewTicker(streamInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.sendProjectSnapshot(ctx, req.Msg, stream); err != nil {
				return err
			}
		}
	}
}

func (s *Server) sendOrgSnapshot(
	ctx context.Context,
	req *organizationv1.StreamOrgWorkloadMetricsRequest,
	stream *connect.ServerStream[organizationv1.StreamWorkloadMetricsResponse],
) error {
	start, end, step := resolveStreamTimeRange(req.GetWindowSeconds(), req.HasStart(), req.GetStart().AsTime(), req.HasEnd(), req.GetEnd().AsTime(), req.GetStepSeconds())

	workload, err := s.GetOrgWorkloadMetrics(ctx, connect.NewRequest(organizationv1.GetOrgWorkloadMetricsRequest_builder{}.Build()))
	if err != nil {
		return err
	}
	ts, err := s.GetOrgWorkloadTimeSeries(ctx, connect.NewRequest(organizationv1.GetOrgWorkloadTimeSeriesRequest_builder{
		Start:       timestamppb.New(start),
		End:         timestamppb.New(end),
		StepSeconds: int32(step.Seconds()),
	}.Build()))
	if err != nil {
		return err
	}

	return stream.Send(organizationv1.StreamWorkloadMetricsResponse_builder{
		Totals:      workload.Msg.GetTotals(),
		Clusters:    workload.Msg.GetClusters(),
		Namespaces:  workload.Msg.GetNamespaces(),
		TimeSeries:  ts.Msg,
		RefreshedAt: timestamppb.Now(),
	}.Build())
}

func (s *Server) sendClusterSnapshot(
	ctx context.Context,
	req *organizationv1.StreamClusterWorkloadMetricsRequest,
	stream *connect.ServerStream[organizationv1.StreamWorkloadMetricsResponse],
) error {
	start, end, step := resolveStreamTimeRange(req.GetWindowSeconds(), req.HasStart(), req.GetStart().AsTime(), req.HasEnd(), req.GetEnd().AsTime(), req.GetStepSeconds())

	workload, err := s.GetClusterWorkloadMetrics(ctx, connect.NewRequest(organizationv1.GetClusterWorkloadMetricsRequest_builder{
		ClusterId: req.GetClusterId(),
	}.Build()))
	if err != nil {
		return err
	}
	ts, err := s.GetClusterWorkloadTimeSeries(ctx, connect.NewRequest(organizationv1.GetClusterWorkloadTimeSeriesRequest_builder{
		ClusterId:   req.GetClusterId(),
		Start:       timestamppb.New(start),
		End:         timestamppb.New(end),
		StepSeconds: int32(step.Seconds()),
	}.Build()))
	if err != nil {
		return err
	}

	return stream.Send(organizationv1.StreamWorkloadMetricsResponse_builder{
		Totals:      workload.Msg.GetTotals(),
		Nodes:       workload.Msg.GetNodes(),
		Namespaces:  workload.Msg.GetNamespaces(),
		TimeSeries:  ts.Msg,
		RefreshedAt: timestamppb.Now(),
	}.Build())
}

func (s *Server) sendProjectSnapshot(
	ctx context.Context,
	req *organizationv1.StreamProjectWorkloadMetricsRequest,
	stream *connect.ServerStream[organizationv1.StreamWorkloadMetricsResponse],
) error {
	start, end, step := resolveStreamTimeRange(req.GetWindowSeconds(), req.HasStart(), req.GetStart().AsTime(), req.HasEnd(), req.GetEnd().AsTime(), req.GetStepSeconds())

	workload, err := s.GetProjectWorkloadMetrics(ctx, connect.NewRequest(organizationv1.GetProjectWorkloadMetricsRequest_builder{
		ProjectId: req.GetProjectId(),
	}.Build()))
	if err != nil {
		return err
	}
	ts, err := s.GetProjectWorkloadTimeSeries(ctx, connect.NewRequest(organizationv1.GetProjectWorkloadTimeSeriesRequest_builder{
		ProjectId:   req.GetProjectId(),
		Start:       timestamppb.New(start),
		End:         timestamppb.New(end),
		StepSeconds: int32(step.Seconds()),
	}.Build()))
	if err != nil {
		return err
	}

	return stream.Send(organizationv1.StreamWorkloadMetricsResponse_builder{
		Totals:      workload.Msg.GetTotals(),
		Namespaces:  workload.Msg.GetNamespaces(),
		TimeSeries:  ts.Msg,
		RefreshedAt: timestamppb.Now(),
	}.Build())
}

// resolveStreamTimeRange computes start/end/step for a streaming snapshot.
// If windowSeconds > 0, start slides as now−window on each call.
// Otherwise falls back to resolveTimeRange (fixed range or 7-day default).
func resolveStreamTimeRange(windowSeconds int32, hasStart bool, start time.Time, hasEnd bool, end time.Time, stepSeconds int32) (time.Time, time.Time, time.Duration) {
	if windowSeconds > 0 {
		now := time.Now()
		_, _, step := resolveTimeRange(false, time.Time{}, false, time.Time{}, stepSeconds)
		return now.Add(-time.Duration(windowSeconds) * time.Second), now, step
	}
	return resolveTimeRange(hasStart, start, hasEnd, end, stepSeconds)
}
