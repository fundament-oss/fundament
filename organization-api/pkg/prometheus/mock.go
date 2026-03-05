package prometheus

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

// ClusterInfo contains the cluster data needed for mock metric generation.
type ClusterInfo struct {
	ID        string
	Name      string
	NodePools []NodePoolInfo
}

// NodePoolInfo contains node pool data needed for mock metric generation.
type NodePoolInfo struct {
	Name         string
	MachineType  string
	AutoscaleMin int32
	AutoscaleMax int32
}

// MockClient is a mock Prometheus client for local development.
// It generates realistic, sinusoidal mock metrics derived from real cluster
// data in the database. Used for clusters whose prometheus_url is set to "mock".
type MockClient struct {
	listClusters func(ctx context.Context) ([]ClusterInfo, error)
}

// NewMockClient creates a MockClient that uses the provided function to fetch
// cluster data on each query. The function receives the request context, so
// database RLS is applied automatically.
func NewMockClient(listClusters func(ctx context.Context) ([]ClusterInfo, error)) *MockClient {
	return &MockClient{listClusters: listClusters}
}

func (c *MockClient) Query(ctx context.Context, query string, t time.Time) ([]Sample, error) {
	clusters, err := c.filteredClusters(ctx, query)
	if err != nil {
		return nil, err
	}
	return mockGenerate(query, t, clusters), nil
}

func (c *MockClient) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]TimeSeries, error) {
	clusters, err := c.filteredClusters(ctx, query)
	if err != nil {
		return nil, err
	}

	// Build series map: label key → {labels, []DataPoint}
	type series struct {
		labels map[string]string
		points []DataPoint
	}
	seriesMap := make(map[string]*series)

	for t := start; !t.After(end); t = t.Add(step) {
		for _, s := range mockGenerate(query, t, clusters) {
			key := mockLabelsKey(s.Labels)
			if seriesMap[key] == nil {
				seriesMap[key] = &series{labels: s.Labels}
			}
			seriesMap[key].points = append(seriesMap[key].points, DataPoint{Time: t, Value: s.Value})
		}
	}

	result := make([]TimeSeries, 0, len(seriesMap))
	for _, s := range seriesMap {
		result = append(result, TimeSeries{Labels: s.labels, Samples: s.points})
	}
	return result, nil
}

func (c *MockClient) filteredClusters(ctx context.Context, q string) ([]ClusterInfo, error) {
	all, err := c.listClusters(ctx)
	if err != nil {
		return nil, err
	}
	name := mockExtractClusterName(q)
	if name == "" {
		return all, nil
	}
	for _, cl := range all {
		if cl.Name == name {
			return []ClusterInfo{cl}, nil
		}
	}
	return nil, nil
}

// ---- Mock data generation ----

const (
	mockCPUCoresPerNode = 2.0
	mockRAMBytesPerNode = 4.0 * 1073741824.0 // 4 GiB
	mockMaxPodsPerNode  = 110.0
	mockNetBytesPerSec  = 10_000_000.0 // 10 MB/s
	mockPowerBaseWatts  = 200.0
	mockPowerRangeWatts = 50.0
)

var mockDefaultNamespaces = []string{"default", "kube-system", "monitoring"}

type mockNode struct {
	clusterName string
	poolName    string
	name        string
	machineID   string
}

func mockClusterNodes(cl ClusterInfo) []mockNode {
	var result []mockNode
	for _, pool := range cl.NodePools {
		count := int(pool.AutoscaleMin+pool.AutoscaleMax) / 2
		if count < 1 {
			count = 1
		}
		for i := range count {
			result = append(result, mockNode{
				clusterName: cl.Name,
				poolName:    pool.Name,
				name:        fmt.Sprintf("%s-%d", pool.Name, i),
				machineID:   mockNodeMachineID(cl.ID, pool.Name, i),
			})
		}
	}
	if len(result) == 0 {
		result = append(result, mockNode{
			clusterName: cl.Name,
			poolName:    "default",
			name:        "node-0",
			machineID:   mockNodeMachineID(cl.ID, "default", 0),
		})
	}
	return result
}

func mockNodeMachineID(clusterID, poolName string, index int) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s/%s/%d", clusterID, poolName, index)))
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

// mockSinFactor returns a high-amplitude factor (0.10–0.90) with a 2-day period.
// Used for CPU, memory, and network to produce visible swings over a 7-day window.
func mockSinFactor(t time.Time) float64 {
	return 0.5 + 0.4*math.Sin(2*math.Pi*float64(t.Unix())/172800)
}

// mockPodSinFactor returns a low-amplitude factor (0.32–0.38) with a 2-day period.
// Pod counts are relatively stable, so only a small variation is applied.
func mockPodSinFactor(t time.Time) float64 {
	return 0.35 + 0.03*math.Sin(2*math.Pi*float64(t.Unix())/172800)
}

// mockGenerate produces Prometheus samples for the given PromQL query at time t.
func mockGenerate(q string, t time.Time, clusters []ClusterInfo) []Sample {
	byNode := mockHasGroupBy(q, "node")
	byNamespace := mockHasGroupBy(q, "namespace")
	byCluster := mockHasGroupBy(q, "cluster")
	nsFilter := mockExtractNamespaces(q)
	machineIDs := mockExtractMachineIDs(q)

	switch {
	case strings.Contains(q, "metal_machine_allocation_info"):
		return mockMetalAllocationInfo(clusters)
	case strings.Contains(q, "metal_machine_power_usage"):
		return mockMetalPowerUsage(clusters, machineIDs, t)
	case strings.Contains(q, "container_cpu_usage_seconds_total"):
		return mockCPUUsage(clusters, t, byNode, byNamespace, byCluster, nsFilter)
	case strings.Contains(q, "kube_node_status_capacity") && strings.Contains(q, `resource="cpu"`):
		return mockCPUCapacity(clusters, byNode, byCluster)
	case strings.Contains(q, "container_memory_working_set_bytes"):
		return mockMemUsage(clusters, t, byNode, byNamespace, byCluster, nsFilter)
	case strings.Contains(q, "kube_node_status_capacity") && strings.Contains(q, `resource="memory"`):
		return mockMemCapacity(clusters, byNode, byCluster)
	case strings.Contains(q, "kube_pod_info"):
		return mockPodCount(clusters, t, byNode, byNamespace, byCluster, nsFilter)
	case strings.Contains(q, "kube_node_status_capacity") && strings.Contains(q, `resource="pods"`):
		return mockPodCapacity(clusters, byNode, byCluster)
	case strings.Contains(q, "kube_pod_container_resource_requests") && strings.Contains(q, `resource="cpu"`):
		return mockNSDistributed(clusters, mockSinFactor(t)*mockCPUCoresPerNode*0.5, nsFilter)
	case strings.Contains(q, "kube_pod_container_resource_limits") && strings.Contains(q, `resource="cpu"`):
		return mockNSDistributed(clusters, mockSinFactor(t)*mockCPUCoresPerNode*0.8, nsFilter)
	case strings.Contains(q, "kube_pod_container_resource_requests") && strings.Contains(q, `resource="memory"`):
		return mockNSDistributed(clusters, mockSinFactor(t)*mockRAMBytesPerNode*0.5, nsFilter)
	case strings.Contains(q, "kube_pod_container_resource_limits") && strings.Contains(q, `resource="memory"`):
		return mockNSDistributed(clusters, mockSinFactor(t)*mockRAMBytesPerNode*0.8, nsFilter)
	case strings.Contains(q, "container_network_receive_bytes_total"):
		return mockNSDistributed(clusters, mockNetBytesPerSec, nsFilter)
	case strings.Contains(q, "container_network_transmit_bytes_total"):
		return mockNSDistributed(clusters, mockNetBytesPerSec*0.5, nsFilter)
	default:
		return nil
	}
}

func mockMetalAllocationInfo(clusters []ClusterInfo) []Sample {
	var result []Sample
	for _, cl := range clusters {
		tag := fmt.Sprintf("shoot--garden-mock--%s", cl.Name)
		poolTypes := make(map[string]string, len(cl.NodePools))
		for _, p := range cl.NodePools {
			poolTypes[p.Name] = p.MachineType
		}
		for _, n := range mockClusterNodes(cl) {
			size := poolTypes[n.poolName]
			if size == "" {
				size = "mock-standard"
			}
			result = append(result, Sample{
				Labels: map[string]string{
					"machineid":   n.machineID,
					"machinename": n.name,
					"size":        size,
					"state":       "Allocated",
					"clusterTag":  tag,
				},
				Value: 1,
			})
		}
	}
	return result
}

func mockMetalPowerUsage(clusters []ClusterInfo, machineIDs map[string]bool, t time.Time) []Sample {
	power := mockPowerBaseWatts + mockPowerRangeWatts*math.Sin(2*math.Pi*float64(t.Unix())/3600)
	var result []Sample
	for _, cl := range clusters {
		for _, n := range mockClusterNodes(cl) {
			if machineIDs != nil && !machineIDs[n.machineID] {
				continue
			}
			result = append(result, Sample{
				Labels: map[string]string{"machineid": n.machineID},
				Value:  power,
			})
		}
	}
	return result
}

func mockCPUUsage(clusters []ClusterInfo, t time.Time, byNode, byNamespace, byCluster bool, nsFilter []string) []Sample {
	f := mockSinFactor(t)
	switch {
	case byNode:
		var result []Sample
		for _, cl := range clusters {
			for _, n := range mockClusterNodes(cl) {
				result = append(result, Sample{
					Labels: map[string]string{"node": n.name, "cluster": n.clusterName},
					Value:  f * mockCPUCoresPerNode,
				})
			}
		}
		return result
	case byNamespace:
		return mockNSDistributed(clusters, f*mockCPUCoresPerNode, nsFilter)
	case byCluster:
		return mockPerCluster(clusters, func(cl ClusterInfo) float64 {
			return f * mockCPUCoresPerNode * float64(len(mockClusterNodes(cl)))
		})
	default:
		return mockScalar(clusters, func(cl ClusterInfo) float64 {
			return f * mockCPUCoresPerNode * float64(len(mockClusterNodes(cl)))
		})
	}
}

func mockCPUCapacity(clusters []ClusterInfo, byNode, byCluster bool) []Sample {
	switch {
	case byNode:
		var result []Sample
		for _, cl := range clusters {
			for _, n := range mockClusterNodes(cl) {
				result = append(result, Sample{
					Labels: map[string]string{"node": n.name, "cluster": n.clusterName},
					Value:  mockCPUCoresPerNode,
				})
			}
		}
		return result
	case byCluster:
		return mockPerCluster(clusters, func(cl ClusterInfo) float64 {
			return mockCPUCoresPerNode * float64(len(mockClusterNodes(cl)))
		})
	default:
		return mockScalar(clusters, func(cl ClusterInfo) float64 {
			return mockCPUCoresPerNode * float64(len(mockClusterNodes(cl)))
		})
	}
}

func mockMemUsage(clusters []ClusterInfo, t time.Time, byNode, byNamespace, byCluster bool, nsFilter []string) []Sample {
	f := mockSinFactor(t)
	switch {
	case byNode:
		var result []Sample
		for _, cl := range clusters {
			for _, n := range mockClusterNodes(cl) {
				result = append(result, Sample{
					Labels: map[string]string{"node": n.name, "cluster": n.clusterName},
					Value:  f * mockRAMBytesPerNode,
				})
			}
		}
		return result
	case byNamespace:
		return mockNSDistributed(clusters, f*mockRAMBytesPerNode, nsFilter)
	case byCluster:
		return mockPerCluster(clusters, func(cl ClusterInfo) float64 {
			return f * mockRAMBytesPerNode * float64(len(mockClusterNodes(cl)))
		})
	default:
		return mockScalar(clusters, func(cl ClusterInfo) float64 {
			return f * mockRAMBytesPerNode * float64(len(mockClusterNodes(cl)))
		})
	}
}

func mockMemCapacity(clusters []ClusterInfo, byNode, byCluster bool) []Sample {
	switch {
	case byNode:
		var result []Sample
		for _, cl := range clusters {
			for _, n := range mockClusterNodes(cl) {
				result = append(result, Sample{
					Labels: map[string]string{"node": n.name, "cluster": n.clusterName},
					Value:  mockRAMBytesPerNode,
				})
			}
		}
		return result
	case byCluster:
		return mockPerCluster(clusters, func(cl ClusterInfo) float64 {
			return mockRAMBytesPerNode * float64(len(mockClusterNodes(cl)))
		})
	default:
		return mockScalar(clusters, func(cl ClusterInfo) float64 {
			return mockRAMBytesPerNode * float64(len(mockClusterNodes(cl)))
		})
	}
}

func mockPodCount(clusters []ClusterInfo, t time.Time, byNode, byNamespace, byCluster bool, nsFilter []string) []Sample {
	podsPerNode := math.Round(10 + 10*mockPodSinFactor(t))
	switch {
	case byNode:
		var result []Sample
		for _, cl := range clusters {
			for _, n := range mockClusterNodes(cl) {
				result = append(result, Sample{
					Labels: map[string]string{"node": n.name, "cluster": n.clusterName},
					Value:  podsPerNode,
				})
			}
		}
		return result
	case byNamespace:
		return mockNSDistributed(clusters, podsPerNode, nsFilter)
	case byCluster:
		return mockPerCluster(clusters, func(cl ClusterInfo) float64 {
			return podsPerNode * float64(len(mockClusterNodes(cl)))
		})
	default:
		return mockScalar(clusters, func(cl ClusterInfo) float64 {
			return podsPerNode * float64(len(mockClusterNodes(cl)))
		})
	}
}

func mockPodCapacity(clusters []ClusterInfo, byNode, byCluster bool) []Sample {
	switch {
	case byNode:
		var result []Sample
		for _, cl := range clusters {
			for _, n := range mockClusterNodes(cl) {
				result = append(result, Sample{
					Labels: map[string]string{"node": n.name, "cluster": n.clusterName},
					Value:  mockMaxPodsPerNode,
				})
			}
		}
		return result
	case byCluster:
		return mockPerCluster(clusters, func(cl ClusterInfo) float64 {
			return mockMaxPodsPerNode * float64(len(mockClusterNodes(cl)))
		})
	default:
		return mockScalar(clusters, func(cl ClusterInfo) float64 {
			return mockMaxPodsPerNode * float64(len(mockClusterNodes(cl)))
		})
	}
}

// ---- Aggregation helpers ----

func mockPerCluster(clusters []ClusterInfo, valueFn func(ClusterInfo) float64) []Sample {
	var result []Sample
	for _, cl := range clusters {
		v := valueFn(cl)
		if v == 0 {
			continue
		}
		result = append(result, Sample{
			Labels: map[string]string{"cluster": cl.Name},
			Value:  v,
		})
	}
	return result
}

func mockScalar(clusters []ClusterInfo, valueFn func(ClusterInfo) float64) []Sample {
	var total float64
	for _, cl := range clusters {
		total += valueFn(cl)
	}
	if total == 0 {
		return nil
	}
	return []Sample{{Labels: map[string]string{}, Value: total}}
}

func mockNSDistributed(clusters []ClusterInfo, perNodeValue float64, nsFilter []string) []Sample {
	nss := mockDefaultNamespaces
	if len(nsFilter) > 0 {
		nss = nsFilter
	}
	if len(nss) == 0 {
		return nil
	}
	var totalNodes int
	for _, cl := range clusters {
		totalNodes += len(mockClusterNodes(cl))
	}
	if totalNodes == 0 {
		return nil
	}
	total := perNodeValue * float64(totalNodes)
	share := total / float64(len(nss))
	result := make([]Sample, 0, len(nss))
	for _, ns := range nss {
		result = append(result, Sample{
			Labels: map[string]string{"namespace": ns},
			Value:  share,
		})
	}
	return result
}

// ---- PromQL filter/groupby helpers ----

var (
	mockReClusterLabel    = regexp.MustCompile(`cluster="([^"]+)"`)
	mockReClusterTagLabel = regexp.MustCompile(`clusterTag=~"[^"]*--([^"\\|$]+)`)
	mockReNamespaceRegex  = regexp.MustCompile(`namespace=~"([^"]+)"`)
	mockReMachineIDRegex  = regexp.MustCompile(`machineid=~"([^"]+)"`)
	mockReGroupBy         = regexp.MustCompile(`\bby\s*\(\s*([^)]+)\s*\)`)
)

func mockExtractClusterName(q string) string {
	if m := mockReClusterLabel.FindStringSubmatch(q); m != nil {
		return m[1]
	}
	if m := mockReClusterTagLabel.FindStringSubmatch(q); m != nil {
		return m[1]
	}
	return ""
}

func mockExtractNamespaces(q string) []string {
	m := mockReNamespaceRegex.FindStringSubmatch(q)
	if m == nil {
		return nil
	}
	return strings.Split(m[1], "|")
}

func mockExtractMachineIDs(q string) map[string]bool {
	m := mockReMachineIDRegex.FindStringSubmatch(q)
	if m == nil {
		return nil
	}
	set := make(map[string]bool)
	for _, id := range strings.Split(m[1], "|") {
		if id != "" {
			set[id] = true
		}
	}
	return set
}

func mockHasGroupBy(q, label string) bool {
	m := mockReGroupBy.FindStringSubmatch(q)
	if m == nil {
		return false
	}
	for _, f := range strings.Split(m[1], ",") {
		if strings.TrimSpace(f) == label {
			return true
		}
	}
	return false
}

func mockLabelsKey(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+labels[k])
	}
	return strings.Join(parts, ",")
}
