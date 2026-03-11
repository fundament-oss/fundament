package organization

import (
	"testing"
	"time"

	prom "github.com/fundament-oss/fundament/organization-api/pkg/prometheus"
)

// ---- resolveTimeRange ----

func TestResolveTimeRange_Defaults(t *testing.T) {
	start, end, step := resolveTimeRange(false, time.Time{}, false, time.Time{}, 0)

	if end.Before(start) {
		t.Errorf("end %v is before start %v", end, start)
	}
	wantDuration := 7 * 24 * time.Hour
	if got := end.Sub(start).Round(time.Second); got != wantDuration {
		t.Errorf("default range = %v, want %v", got, wantDuration)
	}
	if step != 300*time.Second {
		t.Errorf("default step = %v, want 300s", step)
	}
}

func TestResolveTimeRange_CustomStep(t *testing.T) {
	_, _, step := resolveTimeRange(false, time.Time{}, false, time.Time{}, 60)
	if step != 60*time.Second {
		t.Errorf("got step %v, want 60s", step)
	}
}

func TestResolveTimeRange_ExplicitRange(t *testing.T) {
	s := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := s.Add(24 * time.Hour)
	start, end, _ := resolveTimeRange(true, s, true, e, 0)
	if !start.Equal(s) || !end.Equal(e) {
		t.Errorf("got start=%v end=%v, want %v %v", start, end, s, e)
	}
}

// ---- promEscapeLabelValue ----

func TestPromEscapeLabelValue(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`simple`, `simple`},
		{`with"quote`, `with\"quote`},
		{`with\backslash`, `with\\backslash`},
		{`both\"`, `both\\\"`},
	}
	for _, tc := range tests {
		if got := promEscapeLabelValue(tc.input); got != tc.want {
			t.Errorf("promEscapeLabelValue(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---- buildNamespaceFilter ----

func TestBuildNamespaceFilter_Single(t *testing.T) {
	got := buildNamespaceFilter([]string{"default"})
	want := `namespace=~"default"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildNamespaceFilter_Multiple(t *testing.T) {
	got := buildNamespaceFilter([]string{"default", "kube-system"})
	want := `namespace=~"default|kube-system"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildNamespaceFilter_Empty(t *testing.T) {
	got := buildNamespaceFilter(nil)
	// Must return the sentinel that matches nothing (not a valid DNS name)
	if got != `namespace="_"` {
		t.Errorf("got %q, want sentinel", got)
	}
}

// ---- mergeSamples ----

func TestMergeSamples_SumsAcrossClusters(t *testing.T) {
	all := [][]prom.Sample{
		{{Labels: map[string]string{"namespace": "default"}, Value: 1.0}},
		{{Labels: map[string]string{"namespace": "default"}, Value: 2.0}},
		{{Labels: map[string]string{"namespace": "kube-system"}, Value: 3.0}},
	}
	got := mergeSamples(all, "namespace")
	sums := make(map[string]float64)
	for _, s := range got {
		sums[s.Labels["namespace"]] = s.Value
	}
	if sums["default"] != 3.0 {
		t.Errorf("default: got %v, want 3.0", sums["default"])
	}
	if sums["kube-system"] != 3.0 {
		t.Errorf("kube-system: got %v, want 3.0", sums["kube-system"])
	}
}

func TestMergeSamples_Empty(t *testing.T) {
	got := mergeSamples(nil, "namespace")
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

// ---- sumTimeSeries ----

func TestSumTimeSeries_SumsAcrossClusters(t *testing.T) {
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(5 * time.Minute)

	a := []prom.TimeSeries{{Samples: []prom.DataPoint{{Time: t0, Value: 1}, {Time: t1, Value: 2}}}}
	b := []prom.TimeSeries{{Samples: []prom.DataPoint{{Time: t0, Value: 3}, {Time: t1, Value: 4}}}}

	got := sumTimeSeries([][]prom.TimeSeries{a, b})
	if len(got) != 1 {
		t.Fatalf("expected 1 series, got %d", len(got))
	}
	pts := got[0].Samples
	if len(pts) != 2 {
		t.Fatalf("expected 2 points, got %d", len(pts))
	}
	// Sorted by time: t0 first
	if pts[0].Value != 4 || pts[1].Value != 6 {
		t.Errorf("got values [%v %v], want [4 6]", pts[0].Value, pts[1].Value)
	}
}

func TestSumTimeSeries_Empty(t *testing.T) {
	if got := sumTimeSeries(nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// ---- buildNodeMetrics ----

func TestBuildNodeMetrics(t *testing.T) {
	cpu := []prom.Sample{{Labels: map[string]string{"node": "n1"}, Value: 1.0}}
	cpuTot := []prom.Sample{{Labels: map[string]string{"node": "n1"}, Value: 2.0}}
	mem := []prom.Sample{{Labels: map[string]string{"node": "n1"}, Value: 1073741824.0}} // 1 GiB
	memTot := []prom.Sample{{Labels: map[string]string{"node": "n1"}, Value: 4294967296.0}}
	pods := []prom.Sample{{Labels: map[string]string{"node": "n1"}, Value: 10}}
	podsTot := []prom.Sample{{Labels: map[string]string{"node": "n1"}, Value: 110}}

	nodes := buildNodeMetrics(cpu, cpuTot, mem, memTot, pods, podsTot)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	n := nodes[0]
	if n.GetNode() != "n1" {
		t.Errorf("node name = %q, want n1", n.GetNode())
	}
	if n.GetCpu().GetUsed() != 1.0 || n.GetCpu().GetTotal() != 2.0 {
		t.Errorf("cpu = %v/%v, want 1/2", n.GetCpu().GetUsed(), n.GetCpu().GetTotal())
	}
	// 1 GiB / bytesPerGiB = 1.0
	if n.GetMemory().GetUsed() != 1.0 {
		t.Errorf("memory used = %v, want 1.0 GiB", n.GetMemory().GetUsed())
	}
}

// ---- buildNamespaceMetrics ----

func TestBuildNamespaceMetrics(t *testing.T) {
	ns := "default"
	cpu := []prom.Sample{{Labels: map[string]string{"namespace": ns}, Value: 0.5}}
	mem := []prom.Sample{{Labels: map[string]string{"namespace": ns}, Value: 2 * bytesPerGiB}}
	pods := []prom.Sample{{Labels: map[string]string{"namespace": ns}, Value: 5}}
	empty := []prom.Sample{}

	result := buildNamespaceMetrics(cpu, mem, pods, empty, empty, empty, empty, empty, empty)
	if len(result) != 1 {
		t.Fatalf("expected 1 namespace, got %d", len(result))
	}
	r := result[0]
	if r.GetNamespace() != ns {
		t.Errorf("namespace = %q, want %q", r.GetNamespace(), ns)
	}
	if r.GetCpuCores() != 0.5 {
		t.Errorf("cpu = %v, want 0.5", r.GetCpuCores())
	}
	if r.GetMemoryGib() != 2.0 {
		t.Errorf("memory = %v, want 2.0", r.GetMemoryGib())
	}
	if r.GetPods() != 5 {
		t.Errorf("pods = %v, want 5", r.GetPods())
	}
}
