package organization

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// -- Cluster-level RPCs --

func (s *Server) GetClusterWorkloadMetrics(
	ctx context.Context,
	req *connect.Request[organizationv1.GetClusterWorkloadMetricsRequest],
) (*connect.Response[organizationv1.GetClusterWorkloadMetricsResponse], error) {
	clusterID := uuid.MustParse(req.Msg.GetClusterId())

	if err := s.checkPermission(ctx, authz.CanView(), authz.Cluster(clusterID)); err != nil {
		return nil, err
	}

	cluster, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{ID: clusterID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get cluster: %w", err))
	}

	now := time.Now()
	cf := clusterFilter(cluster.Name)

	cpuUsed, err := querySingleValue(ctx, s.k8sPromClient, fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{container!="",%s}[5m]))`, cf), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query cpu used: %w", err))
	}
	cpuTotal, err := querySingleValue(ctx, s.k8sPromClient, fmt.Sprintf(`sum(kube_node_status_capacity{resource="cpu",%s})`, cf), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query cpu total: %w", err))
	}
	memUsed, err := querySingleValue(ctx, s.k8sPromClient, fmt.Sprintf(`sum(container_memory_working_set_bytes{container!="",%s})`, cf), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query mem used: %w", err))
	}
	memTotal, err := querySingleValue(ctx, s.k8sPromClient, fmt.Sprintf(`sum(kube_node_status_capacity{resource="memory",%s})`, cf), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query mem total: %w", err))
	}
	podsUsed, err := querySingleValue(ctx, s.k8sPromClient, fmt.Sprintf(`count(kube_pod_info{%s})`, cf), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query pods used: %w", err))
	}
	podsTotal, err := querySingleValue(ctx, s.k8sPromClient, fmt.Sprintf(`sum(kube_node_status_capacity{resource="pods",%s})`, cf), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query pods total: %w", err))
	}

	totals := organizationv1.ResourceUsageInfo_builder{
		Cpu:    makeResourceUsage(cpuUsed, cpuTotal, "cores"),
		Memory: makeResourceUsage(memUsed/bytesPerGiB, memTotal/bytesPerGiB, "GiB"),
		Pods:   makeResourceUsage(podsUsed, podsTotal, "pods"),
	}.Build()

	// Per-node breakdown
	nodeCPUUsed, err := s.k8sPromClient.Query(ctx, fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{container!="",%s}[5m])) by (node)`, cf), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-node cpu used: %w", err))
	}
	nodeCPUTotal, err := s.k8sPromClient.Query(ctx, fmt.Sprintf(`sum(kube_node_status_capacity{resource="cpu",%s}) by (node)`, cf), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-node cpu total: %w", err))
	}
	nodeMemUsed, err := s.k8sPromClient.Query(ctx, fmt.Sprintf(`sum(container_memory_working_set_bytes{container!="",%s}) by (node)`, cf), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-node mem used: %w", err))
	}
	nodeMemTotal, err := s.k8sPromClient.Query(ctx, fmt.Sprintf(`sum(kube_node_status_capacity{resource="memory",%s}) by (node)`, cf), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-node mem total: %w", err))
	}
	nodePodsUsed, err := s.k8sPromClient.Query(ctx, fmt.Sprintf(`count(kube_pod_info{%s}) by (node)`, cf), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-node pods used: %w", err))
	}
	nodePodsTotal, err := s.k8sPromClient.Query(ctx, fmt.Sprintf(`sum(kube_node_status_capacity{resource="pods",%s}) by (node)`, cf), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-node pods total: %w", err))
	}

	// Per-namespace breakdown
	nsCPU, err := s.k8sPromClient.Query(ctx, fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{container!="",%s}[5m])) by (namespace)`, cf), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace cpu: %w", err))
	}
	nsMem, err := s.k8sPromClient.Query(ctx, fmt.Sprintf(`sum(container_memory_working_set_bytes{container!="",%s}) by (namespace)`, cf), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace mem: %w", err))
	}
	nsPods, err := s.k8sPromClient.Query(ctx, fmt.Sprintf(`count(kube_pod_info{%s}) by (namespace)`, cf), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace pods: %w", err))
	}
	nsCPUReq, err := s.k8sPromClient.Query(ctx, fmt.Sprintf(`sum(kube_pod_container_resource_requests{resource="cpu",%s}) by (namespace)`, cf), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace cpu requests: %w", err))
	}
	nsCPULim, err := s.k8sPromClient.Query(ctx, fmt.Sprintf(`sum(kube_pod_container_resource_limits{resource="cpu",%s}) by (namespace)`, cf), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace cpu limits: %w", err))
	}
	nsMemReq, err := s.k8sPromClient.Query(ctx, fmt.Sprintf(`sum(kube_pod_container_resource_requests{resource="memory",%s}) by (namespace)`, cf), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace mem requests: %w", err))
	}
	nsMemLim, err := s.k8sPromClient.Query(ctx, fmt.Sprintf(`sum(kube_pod_container_resource_limits{resource="memory",%s}) by (namespace)`, cf), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace mem limits: %w", err))
	}
	nsNetRx, err := s.k8sPromClient.Query(ctx, fmt.Sprintf(`sum(rate(container_network_receive_bytes_total{%s}[5m])) by (namespace)`, cf), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace net rx: %w", err))
	}
	nsNetTx, err := s.k8sPromClient.Query(ctx, fmt.Sprintf(`sum(rate(container_network_transmit_bytes_total{%s}[5m])) by (namespace)`, cf), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace net tx: %w", err))
	}

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

	cluster, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{ID: clusterID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get cluster: %w", err))
	}

	start, end, step := resolveTimeRange(req.Msg.HasStart(), req.Msg.GetStart().AsTime(), req.Msg.HasEnd(), req.Msg.GetEnd().AsTime(), req.Msg.GetStepSeconds())
	cf := clusterFilter(cluster.Name)

	cpuSeries, err := s.k8sPromClient.QueryRange(ctx, fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{container!="",%s}[5m]))`, cf), start, end, step)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query cpu time-series: %w", err))
	}
	memSeries, err := s.k8sPromClient.QueryRange(ctx, fmt.Sprintf(`sum(container_memory_working_set_bytes{container!="",%s})`, cf), start, end, step)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query mem time-series: %w", err))
	}
	podSeries, err := s.k8sPromClient.QueryRange(ctx, fmt.Sprintf(`count(kube_pod_info{%s})`, cf), start, end, step)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query pod time-series: %w", err))
	}
	netRxSeries, err := s.k8sPromClient.QueryRange(ctx, fmt.Sprintf(`sum(rate(container_network_receive_bytes_total{%s}[5m]))`, cf), start, end, step)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query net rx time-series: %w", err))
	}
	netTxSeries, err := s.k8sPromClient.QueryRange(ctx, fmt.Sprintf(`sum(rate(container_network_transmit_bytes_total{%s}[5m]))`, cf), start, end, step)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query net tx time-series: %w", err))
	}

	return connect.NewResponse(organizationv1.GetWorkloadTimeSeriesResponse_builder{
		CpuCores:           timeSeriesFirstToProto(cpuSeries, 1),
		MemoryGib:          timeSeriesFirstToProto(memSeries, 1.0/bytesPerGiB),
		PodCount:           timeSeriesFirstToProto(podSeries, 1),
		NetworkReceiveMbS:  timeSeriesFirstToProto(netRxSeries, 1.0/bytesPerMB),
		NetworkTransmitMbS: timeSeriesFirstToProto(netTxSeries, 1.0/bytesPerMB),
	}.Build()), nil
}

func (s *Server) GetClusterInfraMetrics(
	ctx context.Context,
	req *connect.Request[organizationv1.GetClusterInfraMetricsRequest],
) (*connect.Response[organizationv1.GetInfraMetricsResponse], error) {
	clusterID := uuid.MustParse(req.Msg.GetClusterId())

	if err := s.checkPermission(ctx, authz.CanView(), authz.Cluster(clusterID)); err != nil {
		return nil, err
	}

	cluster, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{ID: clusterID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get cluster: %w", err))
	}

	return s.infraMetrics(ctx, fmt.Sprintf(`metal_machine_allocation_info{clusterTag=~"shoot--.*--%s"}`, cluster.Name))
}

// -- Org-level RPCs --

func (s *Server) GetOrgWorkloadMetrics(
	ctx context.Context,
	_ *connect.Request[organizationv1.GetOrgWorkloadMetricsRequest],
) (*connect.Response[organizationv1.GetOrgWorkloadMetricsResponse], error) {
	now := time.Now()

	cpuUsed, err := querySingleValue(ctx, s.k8sPromClient, `sum(rate(container_cpu_usage_seconds_total{container!=""}[5m]))`, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query cpu used: %w", err))
	}
	cpuTotal, err := querySingleValue(ctx, s.k8sPromClient, `sum(kube_node_status_capacity{resource="cpu"})`, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query cpu total: %w", err))
	}
	memUsed, err := querySingleValue(ctx, s.k8sPromClient, `sum(container_memory_working_set_bytes{container!=""})`, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query mem used: %w", err))
	}
	memTotal, err := querySingleValue(ctx, s.k8sPromClient, `sum(kube_node_status_capacity{resource="memory"})`, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query mem total: %w", err))
	}
	podsUsed, err := querySingleValue(ctx, s.k8sPromClient, `count(kube_pod_info)`, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query pods used: %w", err))
	}
	podsTotal, err := querySingleValue(ctx, s.k8sPromClient, `sum(kube_node_status_capacity{resource="pods"})`, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query pods total: %w", err))
	}

	totals := organizationv1.ResourceUsageInfo_builder{
		Cpu:    makeResourceUsage(cpuUsed, cpuTotal, "cores"),
		Memory: makeResourceUsage(memUsed/bytesPerGiB, memTotal/bytesPerGiB, "GiB"),
		Pods:   makeResourceUsage(podsUsed, podsTotal, "pods"),
	}.Build()

	// Per-cluster breakdown requires a "cluster" label in Prometheus (standard in
	// federated setups). Returns empty list when label is absent.
	clusterCPU, err := s.k8sPromClient.Query(ctx, `sum(rate(container_cpu_usage_seconds_total{container!=""}[5m])) by (cluster)`, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-cluster cpu: %w", err))
	}
	clusterMem, err := s.k8sPromClient.Query(ctx, `sum(container_memory_working_set_bytes{container!=""}) by (cluster)`, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-cluster mem: %w", err))
	}
	clusterPods, err := s.k8sPromClient.Query(ctx, `count(kube_pod_info) by (cluster)`, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-cluster pods: %w", err))
	}
	clusterCPUTotal, err := s.k8sPromClient.Query(ctx, `sum(kube_node_status_capacity{resource="cpu"}) by (cluster)`, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-cluster cpu total: %w", err))
	}
	clusterMemTotal, err := s.k8sPromClient.Query(ctx, `sum(kube_node_status_capacity{resource="memory"}) by (cluster)`, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-cluster mem total: %w", err))
	}
	clusterPodsTotal, err := s.k8sPromClient.Query(ctx, `sum(kube_node_status_capacity{resource="pods"}) by (cluster)`, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-cluster pods total: %w", err))
	}

	// Per-namespace breakdown across all clusters
	nsCPU, err := s.k8sPromClient.Query(ctx, `sum(rate(container_cpu_usage_seconds_total{container!=""}[5m])) by (namespace)`, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace cpu: %w", err))
	}
	nsMem, err := s.k8sPromClient.Query(ctx, `sum(container_memory_working_set_bytes{container!=""}) by (namespace)`, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace mem: %w", err))
	}
	nsPods, err := s.k8sPromClient.Query(ctx, `count(kube_pod_info) by (namespace)`, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace pods: %w", err))
	}
	nsCPUReq, err := s.k8sPromClient.Query(ctx, `sum(kube_pod_container_resource_requests{resource="cpu"}) by (namespace)`, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace cpu requests: %w", err))
	}
	nsCPULim, err := s.k8sPromClient.Query(ctx, `sum(kube_pod_container_resource_limits{resource="cpu"}) by (namespace)`, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace cpu limits: %w", err))
	}
	nsMemReq, err := s.k8sPromClient.Query(ctx, `sum(kube_pod_container_resource_requests{resource="memory"}) by (namespace)`, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace mem requests: %w", err))
	}
	nsMemLim, err := s.k8sPromClient.Query(ctx, `sum(kube_pod_container_resource_limits{resource="memory"}) by (namespace)`, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace mem limits: %w", err))
	}
	nsNetRx, err := s.k8sPromClient.Query(ctx, `sum(rate(container_network_receive_bytes_total[5m])) by (namespace)`, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace net rx: %w", err))
	}
	nsNetTx, err := s.k8sPromClient.Query(ctx, `sum(rate(container_network_transmit_bytes_total[5m])) by (namespace)`, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace net tx: %w", err))
	}

	return connect.NewResponse(organizationv1.GetOrgWorkloadMetricsResponse_builder{
		Totals:     totals,
		Clusters:   buildClusterSummaries(clusterCPU, clusterCPUTotal, clusterMem, clusterMemTotal, clusterPods, clusterPodsTotal),
		Namespaces: buildNamespaceMetrics(nsCPU, nsMem, nsPods, nsCPUReq, nsCPULim, nsMemReq, nsMemLim, nsNetRx, nsNetTx),
	}.Build()), nil
}

func (s *Server) GetOrgWorkloadTimeSeries(
	ctx context.Context,
	req *connect.Request[organizationv1.GetOrgWorkloadTimeSeriesRequest],
) (*connect.Response[organizationv1.GetWorkloadTimeSeriesResponse], error) {
	start, end, step := resolveTimeRange(req.Msg.HasStart(), req.Msg.GetStart().AsTime(), req.Msg.HasEnd(), req.Msg.GetEnd().AsTime(), req.Msg.GetStepSeconds())

	cpuSeries, err := s.k8sPromClient.QueryRange(ctx, `sum(rate(container_cpu_usage_seconds_total{container!=""}[5m]))`, start, end, step)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query cpu time-series: %w", err))
	}
	memSeries, err := s.k8sPromClient.QueryRange(ctx, `sum(container_memory_working_set_bytes{container!=""})`, start, end, step)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query mem time-series: %w", err))
	}
	podSeries, err := s.k8sPromClient.QueryRange(ctx, `count(kube_pod_info)`, start, end, step)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query pod time-series: %w", err))
	}
	netRxSeries, err := s.k8sPromClient.QueryRange(ctx, `sum(rate(container_network_receive_bytes_total[5m]))`, start, end, step)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query net rx time-series: %w", err))
	}
	netTxSeries, err := s.k8sPromClient.QueryRange(ctx, `sum(rate(container_network_transmit_bytes_total[5m]))`, start, end, step)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query net tx time-series: %w", err))
	}

	return connect.NewResponse(organizationv1.GetWorkloadTimeSeriesResponse_builder{
		CpuCores:           timeSeriesFirstToProto(cpuSeries, 1),
		MemoryGib:          timeSeriesFirstToProto(memSeries, 1.0/bytesPerGiB),
		PodCount:           timeSeriesFirstToProto(podSeries, 1),
		NetworkReceiveMbS:  timeSeriesFirstToProto(netRxSeries, 1.0/bytesPerMB),
		NetworkTransmitMbS: timeSeriesFirstToProto(netTxSeries, 1.0/bytesPerMB),
	}.Build()), nil
}

func (s *Server) GetOrgInfraMetrics(
	ctx context.Context,
	_ *connect.Request[organizationv1.GetOrgInfraMetricsRequest],
) (*connect.Response[organizationv1.GetInfraMetricsResponse], error) {
	// No clusterTag filter — returns all machines the metal-stack Prometheus exposes.
	return s.infraMetrics(ctx, `metal_machine_allocation_info`)
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

	nsFilter := buildNamespaceFilter(namespaceNames(namespaces))
	now := time.Now()

	cpuUsed, err := querySingleValue(ctx, s.k8sPromClient, fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{container!="",%s}[5m]))`, nsFilter), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query cpu used: %w", err))
	}
	cpuTotal, err := querySingleValue(ctx, s.k8sPromClient, `sum(kube_node_status_capacity{resource="cpu"})`, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query cpu total: %w", err))
	}
	memUsed, err := querySingleValue(ctx, s.k8sPromClient, fmt.Sprintf(`sum(container_memory_working_set_bytes{container!="",%s})`, nsFilter), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query mem used: %w", err))
	}
	memTotal, err := querySingleValue(ctx, s.k8sPromClient, `sum(kube_node_status_capacity{resource="memory"})`, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query mem total: %w", err))
	}
	podsUsed, err := querySingleValue(ctx, s.k8sPromClient, fmt.Sprintf(`count(kube_pod_info{%s})`, nsFilter), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query pods used: %w", err))
	}
	podsTotal, err := querySingleValue(ctx, s.k8sPromClient, `sum(kube_node_status_capacity{resource="pods"})`, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query pods total: %w", err))
	}

	nsCPU, err := s.k8sPromClient.Query(ctx, fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{container!="",%s}[5m])) by (namespace)`, nsFilter), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace cpu: %w", err))
	}
	nsMem, err := s.k8sPromClient.Query(ctx, fmt.Sprintf(`sum(container_memory_working_set_bytes{container!="",%s}) by (namespace)`, nsFilter), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace mem: %w", err))
	}
	nsPods, err := s.k8sPromClient.Query(ctx, fmt.Sprintf(`count(kube_pod_info{%s}) by (namespace)`, nsFilter), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace pods: %w", err))
	}
	nsCPUReq, err := s.k8sPromClient.Query(ctx, fmt.Sprintf(`sum(kube_pod_container_resource_requests{resource="cpu",%s}) by (namespace)`, nsFilter), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace cpu requests: %w", err))
	}
	nsCPULim, err := s.k8sPromClient.Query(ctx, fmt.Sprintf(`sum(kube_pod_container_resource_limits{resource="cpu",%s}) by (namespace)`, nsFilter), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace cpu limits: %w", err))
	}
	nsMemReq, err := s.k8sPromClient.Query(ctx, fmt.Sprintf(`sum(kube_pod_container_resource_requests{resource="memory",%s}) by (namespace)`, nsFilter), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace mem requests: %w", err))
	}
	nsMemLim, err := s.k8sPromClient.Query(ctx, fmt.Sprintf(`sum(kube_pod_container_resource_limits{resource="memory",%s}) by (namespace)`, nsFilter), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace mem limits: %w", err))
	}
	nsNetRx, err := s.k8sPromClient.Query(ctx, fmt.Sprintf(`sum(rate(container_network_receive_bytes_total{%s}[5m])) by (namespace)`, nsFilter), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace net rx: %w", err))
	}
	nsNetTx, err := s.k8sPromClient.Query(ctx, fmt.Sprintf(`sum(rate(container_network_transmit_bytes_total{%s}[5m])) by (namespace)`, nsFilter), now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query per-namespace net tx: %w", err))
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

	nsFilter := buildNamespaceFilter(namespaceNames(namespaces))
	start, end, step := resolveTimeRange(req.Msg.HasStart(), req.Msg.GetStart().AsTime(), req.Msg.HasEnd(), req.Msg.GetEnd().AsTime(), req.Msg.GetStepSeconds())

	cpuSeries, err := s.k8sPromClient.QueryRange(ctx, fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{container!="",%s}[5m]))`, nsFilter), start, end, step)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query cpu time-series: %w", err))
	}
	memSeries, err := s.k8sPromClient.QueryRange(ctx, fmt.Sprintf(`sum(container_memory_working_set_bytes{container!="",%s})`, nsFilter), start, end, step)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query mem time-series: %w", err))
	}
	podSeries, err := s.k8sPromClient.QueryRange(ctx, fmt.Sprintf(`count(kube_pod_info{%s})`, nsFilter), start, end, step)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query pod time-series: %w", err))
	}
	netRxSeries, err := s.k8sPromClient.QueryRange(ctx, fmt.Sprintf(`sum(rate(container_network_receive_bytes_total{%s}[5m]))`, nsFilter), start, end, step)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query net rx time-series: %w", err))
	}
	netTxSeries, err := s.k8sPromClient.QueryRange(ctx, fmt.Sprintf(`sum(rate(container_network_transmit_bytes_total{%s}[5m]))`, nsFilter), start, end, step)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query net tx time-series: %w", err))
	}

	return connect.NewResponse(organizationv1.GetWorkloadTimeSeriesResponse_builder{
		CpuCores:           timeSeriesFirstToProto(cpuSeries, 1),
		MemoryGib:          timeSeriesFirstToProto(memSeries, 1.0/bytesPerGiB),
		PodCount:           timeSeriesFirstToProto(podSeries, 1),
		NetworkReceiveMbS:  timeSeriesFirstToProto(netRxSeries, 1.0/bytesPerMB),
		NetworkTransmitMbS: timeSeriesFirstToProto(netTxSeries, 1.0/bytesPerMB),
	}.Build()), nil
}

// -- Shared infrastructure helper --

// infraMetrics queries metal-stack machine info and power usage using the given
// machine allocation query. machineQuery should be a PromQL expression that
// returns metal_machine_allocation_info samples.
func (s *Server) infraMetrics(ctx context.Context, machineQuery string) (*connect.Response[organizationv1.GetInfraMetricsResponse], error) {
	now := time.Now()

	machines, err := s.metalPromClient.Query(ctx, machineQuery, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query machines: %w", err))
	}

	if len(machines) == 0 {
		return connect.NewResponse(organizationv1.GetInfraMetricsResponse_builder{}.Build()), nil
	}

	machineIDs := make([]string, 0, len(machines))
	for _, m := range machines {
		if id := m.Labels["machineid"]; id != "" {
			machineIDs = append(machineIDs, id)
		}
	}

	powerByID := make(map[string]float64)
	if len(machineIDs) > 0 {
		powerSamples, err := s.metalPromClient.Query(ctx, buildPowerQuery(machineIDs), now)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("query power: %w", err))
		}
		for _, p := range powerSamples {
			if id := p.Labels["machineid"]; id != "" {
				powerByID[id] = p.Value
			}
		}
	}

	var totalPower float64
	machineInfos := make([]*organizationv1.MachineInfo, 0, len(machines))
	for _, m := range machines {
		id := m.Labels["machineid"]
		power := powerByID[id]
		totalPower += power

		machineInfos = append(machineInfos, organizationv1.MachineInfo_builder{
			Id:         id,
			Name:       m.Labels["machinename"],
			Size:       m.Labels["size"],
			State:      m.Labels["state"],
			PowerWatts: power,
		}.Build())
	}

	return connect.NewResponse(organizationv1.GetInfraMetricsResponse_builder{
		Machines:        machineInfos,
		TotalPowerWatts: totalPower,
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

// buildClusterSummaries combines per-cluster CPU/memory/pod samples into
// ClusterWorkloadSummary messages. Requires a "cluster" label in Prometheus.
func buildClusterSummaries(
	cpuUsed, cpuTotal, memUsed, memTotal, podsUsed, podsTotal []prom.Sample,
) []*organizationv1.ClusterWorkloadSummary {
	type clusterData struct {
		cpuUsed, cpuTotal   float64
		memUsed, memTotal   float64
		podsUsed, podsTotal float64
	}
	clusters := make(map[string]*clusterData)

	ensureCluster := func(name string) *clusterData {
		if clusters[name] == nil {
			clusters[name] = &clusterData{}
		}
		return clusters[name]
	}

	for _, s := range cpuUsed {
		ensureCluster(s.Labels["cluster"]).cpuUsed = s.Value
	}
	for _, s := range cpuTotal {
		ensureCluster(s.Labels["cluster"]).cpuTotal = s.Value
	}
	for _, s := range memUsed {
		ensureCluster(s.Labels["cluster"]).memUsed = s.Value
	}
	for _, s := range memTotal {
		ensureCluster(s.Labels["cluster"]).memTotal = s.Value
	}
	for _, s := range podsUsed {
		ensureCluster(s.Labels["cluster"]).podsUsed = s.Value
	}
	for _, s := range podsTotal {
		ensureCluster(s.Labels["cluster"]).podsTotal = s.Value
	}

	result := make([]*organizationv1.ClusterWorkloadSummary, 0, len(clusters))
	for name, d := range clusters {
		result = append(result, organizationv1.ClusterWorkloadSummary_builder{
			ClusterName: name,
			Cpu:         makeResourceUsage(d.cpuUsed, d.cpuTotal, "cores"),
			Memory:      makeResourceUsage(d.memUsed/bytesPerGiB, d.memTotal/bytesPerGiB, "GiB"),
			Pods:        makeResourceUsage(d.podsUsed, d.podsTotal, "pods"),
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

// clusterFilter returns a PromQL label selector fragment for the given cluster name.
// Assumes a "cluster" label is present in the Prometheus time-series.
func clusterFilter(clusterName string) string {
	return fmt.Sprintf(`cluster="%s"`, clusterName)
}

// buildNamespaceFilter returns a PromQL label selector fragment that matches any
// of the given namespace names: namespace=~"ns1|ns2|ns3".
func buildNamespaceFilter(names []string) string {
	return fmt.Sprintf(`namespace=~"%s"`, strings.Join(names, "|"))
}

// buildPowerQuery returns a PromQL query for machine power usage filtered to the
// given machine IDs.
func buildPowerQuery(machineIDs []string) string {
	return fmt.Sprintf(`metal_machine_power_usage{machineid=~"%s"}`, strings.Join(machineIDs, "|"))
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
	step := 5 * time.Minute
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
