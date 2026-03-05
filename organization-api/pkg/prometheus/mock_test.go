package prometheus

import (
	"context"
	"sort"
	"testing"
	"time"
)

var testCluster = ClusterInfo{
	ID:   "aaaaaaaa-0000-0000-0000-000000000001",
	Name: "my-cluster",
	NodePools: []NodePoolInfo{
		{Name: "workers", MachineType: "c1-medium", AutoscaleMin: 2, AutoscaleMax: 4},
	},
}

var testClusters = []ClusterInfo{testCluster}

// ---- mockExtractClusterName ----

func TestMockExtractClusterName_ClusterLabel(t *testing.T) {
	q := `container_cpu_usage_seconds_total{cluster="my-cluster"}`
	if got := mockExtractClusterName(q); got != "my-cluster" {
		t.Errorf("got %q, want %q", got, "my-cluster")
	}
}

func TestMockExtractClusterName_ClusterTagLabel(t *testing.T) {
	q := `metal_machine_allocation_info{clusterTag=~"shoot--.*--my-cluster$"}`
	if got := mockExtractClusterName(q); got != "my-cluster" {
		t.Errorf("got %q, want %q", got, "my-cluster")
	}
}

func TestMockExtractClusterName_NoCluster(t *testing.T) {
	q := `sum(rate(container_cpu_usage_seconds_total[5m]))`
	if got := mockExtractClusterName(q); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// ---- mockHasGroupBy ----

func TestMockHasGroupBy(t *testing.T) {
	tests := []struct {
		query string
		label string
		want  bool
	}{
		{`sum(...) by (node)`, "node", true},
		{`sum(...) by (namespace)`, "namespace", true},
		{`sum(...) by (node, namespace)`, "node", true},
		{`sum(...) by (node, namespace)`, "namespace", true},
		{`sum(...)`, "node", false},
		{`sum(...) by (cluster)`, "node", false},
	}
	for _, tc := range tests {
		got := mockHasGroupBy(tc.query, tc.label)
		if got != tc.want {
			t.Errorf("mockHasGroupBy(%q, %q) = %v, want %v", tc.query, tc.label, got, tc.want)
		}
	}
}

// ---- mockExtractNamespaces ----

func TestMockExtractNamespaces_Single(t *testing.T) {
	q := `sum(...{namespace=~"kube-system"})`
	got := mockExtractNamespaces(q)
	if len(got) != 1 || got[0] != "kube-system" {
		t.Errorf("got %v, want [kube-system]", got)
	}
}

func TestMockExtractNamespaces_Multiple(t *testing.T) {
	q := `sum(...{namespace=~"default|kube-system|monitoring"})`
	got := mockExtractNamespaces(q)
	want := []string{"default", "kube-system", "monitoring"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("[%d]: got %q, want %q", i, got[i], v)
		}
	}
}

func TestMockExtractNamespaces_None(t *testing.T) {
	q := `sum(rate(container_cpu_usage_seconds_total[5m]))`
	if got := mockExtractNamespaces(q); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

// ---- mockClusterNodes ----

func TestMockClusterNodes_UsesAverageOfMinMax(t *testing.T) {
	cl := ClusterInfo{
		ID:   "test-id",
		Name: "cl",
		NodePools: []NodePoolInfo{
			{Name: "pool", AutoscaleMin: 2, AutoscaleMax: 4},
		},
	}
	nodes := mockClusterNodes(cl)
	// (2+4)/2 = 3
	if len(nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(nodes))
	}
}

func TestMockClusterNodes_FallsBackToOneNodeWhenEmpty(t *testing.T) {
	cl := ClusterInfo{ID: "x", Name: "cl"}
	nodes := mockClusterNodes(cl)
	if len(nodes) != 1 {
		t.Errorf("expected 1 fallback node, got %d", len(nodes))
	}
}

func TestMockClusterNodes_MinimumOneNodePerPool(t *testing.T) {
	cl := ClusterInfo{
		ID:   "x",
		Name: "cl",
		NodePools: []NodePoolInfo{
			{Name: "pool", AutoscaleMin: 0, AutoscaleMax: 0},
		},
	}
	nodes := mockClusterNodes(cl)
	if len(nodes) != 1 {
		t.Errorf("expected 1 node (min 1), got %d", len(nodes))
	}
}

// ---- mockLabelsKey ----

func TestMockLabelsKey_Deterministic(t *testing.T) {
	a := mockLabelsKey(map[string]string{"b": "2", "a": "1"})
	b := mockLabelsKey(map[string]string{"a": "1", "b": "2"})
	if a != b {
		t.Errorf("non-deterministic: %q != %q", a, b)
	}
}

// ---- mockGenerate dispatch ----

func TestMockGenerate_CPUUsage_Scalar(t *testing.T) {
	q := `sum(rate(container_cpu_usage_seconds_total{container!=""}[5m]))`
	samples := mockGenerate(q, time.Now(), testClusters)
	if len(samples) != 1 {
		t.Fatalf("expected 1 scalar sample, got %d", len(samples))
	}
	if samples[0].Value <= 0 {
		t.Errorf("expected positive CPU value, got %v", samples[0].Value)
	}
}

func TestMockGenerate_CPUUsage_ByNode(t *testing.T) {
	q := `sum(rate(container_cpu_usage_seconds_total{container!=""}[5m])) by (node)`
	nodes := mockClusterNodes(testCluster)
	samples := mockGenerate(q, time.Now(), testClusters)
	if len(samples) != len(nodes) {
		t.Errorf("expected %d per-node samples, got %d", len(nodes), len(samples))
	}
	for _, s := range samples {
		if s.Labels["node"] == "" {
			t.Errorf("missing node label: %v", s.Labels)
		}
	}
}

func TestMockGenerate_CPUCapacity_Scalar(t *testing.T) {
	q := `sum(kube_node_status_capacity{resource="cpu"})`
	samples := mockGenerate(q, time.Now(), testClusters)
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(samples))
	}
	nodes := mockClusterNodes(testCluster)
	want := mockCPUCoresPerNode * float64(len(nodes))
	if samples[0].Value != want {
		t.Errorf("got %v, want %v", samples[0].Value, want)
	}
}

func TestMockGenerate_MemCapacity_ByNode(t *testing.T) {
	q := `sum(kube_node_status_capacity{resource="memory"}) by (node)`
	samples := mockGenerate(q, time.Now(), testClusters)
	nodes := mockClusterNodes(testCluster)
	if len(samples) != len(nodes) {
		t.Errorf("expected %d per-node samples, got %d", len(nodes), len(samples))
	}
}

func TestMockGenerate_NamespaceFilter(t *testing.T) {
	q := `sum(container_memory_working_set_bytes{container!="",namespace=~"default|monitoring"}) by (namespace)`
	samples := mockGenerate(q, time.Now(), testClusters)
	names := make([]string, 0, len(samples))
	for _, s := range samples {
		names = append(names, s.Labels["namespace"])
	}
	sort.Strings(names)
	want := []string{"default", "monitoring"}
	if len(names) != len(want) {
		t.Fatalf("got namespaces %v, want %v", names, want)
	}
	for i, v := range want {
		if names[i] != v {
			t.Errorf("[%d]: got %q, want %q", i, names[i], v)
		}
	}
}

func TestMockGenerate_MetalAllocationInfo(t *testing.T) {
	q := `metal_machine_allocation_info`
	samples := mockGenerate(q, time.Now(), testClusters)
	nodes := mockClusterNodes(testCluster)
	if len(samples) != len(nodes) {
		t.Errorf("expected %d machine samples, got %d", len(nodes), len(samples))
	}
	for _, s := range samples {
		if s.Labels["machineid"] == "" {
			t.Errorf("missing machineid label: %v", s.Labels)
		}
		if s.Labels["state"] != "Allocated" {
			t.Errorf("expected state=Allocated, got %q", s.Labels["state"])
		}
	}
}

func TestMockGenerate_UnknownQuery_ReturnsNil(t *testing.T) {
	samples := mockGenerate(`some_unknown_metric`, time.Now(), testClusters)
	if samples != nil {
		t.Errorf("expected nil for unknown query, got %v", samples)
	}
}

// ---- MockClient ----

func makeMockClient(clusters []ClusterInfo) *MockClient {
	return NewMockClient(func(_ context.Context) ([]ClusterInfo, error) {
		return clusters, nil
	})
}

func TestMockClient_Query_FiltersByClusterName(t *testing.T) {
	other := ClusterInfo{ID: "bbbb", Name: "other-cluster"}
	client := makeMockClient([]ClusterInfo{testCluster, other})

	q := `sum(kube_node_status_capacity{resource="cpu",cluster="my-cluster"})`
	samples, err := client.Query(context.Background(), q, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// Only my-cluster nodes, not other-cluster
	nodes := mockClusterNodes(testCluster)
	want := mockCPUCoresPerNode * float64(len(nodes))
	if len(samples) != 1 || samples[0].Value != want {
		t.Errorf("got %v, want 1 sample with value %v", samples, want)
	}
}

func TestMockClient_QueryRange_ReturnsPointsPerStep(t *testing.T) {
	client := makeMockClient(testClusters)

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(10 * time.Minute)
	step := 5 * time.Minute

	series, err := client.QueryRange(context.Background(),
		`sum(rate(container_cpu_usage_seconds_total{container!=""}[5m]))`,
		start, end, step)
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 1 {
		t.Fatalf("expected 1 series, got %d", len(series))
	}
	// start, start+5m, start+10m → 3 points
	if len(series[0].Samples) != 3 {
		t.Errorf("expected 3 data points, got %d", len(series[0].Samples))
	}
}
