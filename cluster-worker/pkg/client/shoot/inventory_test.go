package shoot

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/metadata"
	metadatafake "k8s.io/client-go/metadata/fake"
	k8stesting "k8s.io/client-go/testing"
)

func testNode(name, pool string, ready bool, cpu, memory, pods string) corev1.Node {
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{}},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
				{Type: corev1.NodeReady, Status: status},
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(cpu),
				corev1.ResourceMemory: resource.MustParse(memory),
				corev1.ResourcePods:   resource.MustParse(pods),
			},
		},
	}
	if pool != "" {
		node.Labels[LabelWorkerPool] = pool
	}
	return node
}

func testNamespace(name string, id uuid.UUID) corev1.Namespace {
	return corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   name,
		Labels: map[string]string{LabelNamespaceID: id.String()},
	}}
}

func testPod(namespace, name string) metav1.PartialObjectMetadata {
	return metav1.PartialObjectMetadata{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
	}
}

func TestFoldInventory(t *testing.T) {
	t.Parallel()

	web := uuid.New()
	empty := uuid.New()

	inv := FoldInventory(
		[]corev1.Node{
			testNode("a-1", "workers", true, "3920m", "15Gi", "110"),
			testNode("a-2", "workers", false, "3920m", "15Gi", "110"),
			testNode("b-1", "gpu", true, "8", "64Gi", "58"),
			// Not part of a Gardener worker pool: counted in the totals only.
			testNode("stray", "", true, "500m", "512Mi", "10"),
		},
		[]corev1.Namespace{
			testNamespace("web-prod", web),
			testNamespace("empty", empty),
			{ObjectMeta: metav1.ObjectMeta{Name: "garbled", Labels: map[string]string{LabelNamespaceID: "not-a-uuid"}}},
		},
		[]metav1.PartialObjectMetadata{
			testPod("web-prod", "web-1"),
			testPod("web-prod", "web-2"),
			testPod("kube-system", "coredns"),
			testPod("garbled", "x"),
		},
	)

	assert.Equal(t, int32(4), inv.Nodes)
	assert.Equal(t, int32(3), inv.NodesReady)
	assert.Equal(t, int32(110+110+58+10), inv.PodAllocatable)
	assert.Equal(t, int64(3920+3920+8000+500), inv.CPUAllocatableMilli)
	assert.Equal(t, int64(15*1024+15*1024+64*1024+512), inv.MemoryAllocatableMi)
	assert.Equal(t, int32(4), inv.Pods, "every active pod counts toward the cluster total")

	assert.Equal(t, map[string]PoolInventory{
		"workers": {Nodes: 2, NodesReady: 1},
		"gpu":     {Nodes: 1, NodesReady: 1},
	}, inv.Pools)

	assert.Equal(t, map[uuid.UUID]int32{web: 2, empty: 0}, inv.NamespacePods,
		"a namespace with no pods is reported as zero; one with an unparsable id is skipped")
}

func TestFoldInventory_NodeWithoutReadyCondition(t *testing.T) {
	t.Parallel()

	node := testNode("new", "workers", true, "1", "1Gi", "10")
	node.Status.Conditions = nil

	inv := FoldInventory([]corev1.Node{node}, nil, nil)

	assert.Equal(t, int32(1), inv.Nodes)
	assert.Equal(t, int32(0), inv.NodesReady, "a node that reports no Ready condition is not ready")
	assert.Equal(t, PoolInventory{Nodes: 1}, inv.Pools["workers"])
}

// metadataScheme registers PartialObjectMetadata, as the metadata fake needs.
func metadataScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := metadatafake.NewTestScheme()
	require.NoError(t, metav1.AddMetaToScheme(scheme))
	return scheme
}

func inventoryAccessWith(t *testing.T, cs kubernetes.Interface, md metadata.Interface) *RealShootAccess {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	return &RealShootAccess{
		logger: logger,
		newClient: func(context.Context, uuid.UUID) (kubernetes.Interface, error) {
			return cs, nil
		},
		newAPIExtClient: notWired[apiextensionsclientset.Interface]("apiextensions"),
		newMetadataClient: func(context.Context, uuid.UUID) (metadata.Interface, error) {
			return md, nil
		},
	}
}

func TestReadInventory(t *testing.T) {
	t.Parallel()

	nsID := uuid.New()
	node := testNode("a-1", "workers", true, "2", "8Gi", "110")
	ns := testNamespace("team-a", nsID)
	unmanaged := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}
	cs := fake.NewClientset(&node, &ns, &unmanaged)

	pod1 := testPod("team-a", "api-1")
	pod2 := testPod("default", "debug")
	md := metadatafake.NewSimpleMetadataClient(metadataScheme(t), &pod1, &pod2)

	r := inventoryAccessWith(t, cs, md)
	inv, err := r.ReadInventory(t.Context(), uuid.New())
	require.NoError(t, err)

	assert.Equal(t, int32(1), inv.Nodes)
	assert.Equal(t, int32(2), inv.Pods)
	assert.Equal(t, map[uuid.UUID]int32{nsID: 1}, inv.NamespacePods,
		"only namespaces carrying the fundament id label are reported")

	// The fakes do not evaluate field selectors, so assert what was asked: pods
	// as metadata only, without terminal phases, and every list from the
	// watch cache.
	var podList k8stesting.ListAction
	for _, a := range md.Actions() {
		if la, ok := a.(k8stesting.ListAction); ok && a.GetResource().Resource == "pods" {
			podList = la
		}
	}
	require.NotNil(t, podList, "pods are listed through the metadata client")
	assert.Equal(t, "status.phase!=Failed,status.phase!=Succeeded", podList.GetListRestrictions().Fields.String())

	for _, a := range cs.Actions() {
		la, ok := a.(k8stesting.ListActionImpl)
		if !ok {
			continue
		}
		assert.Equal(t, "0", la.ListOptions.ResourceVersion, "%s served from the watch cache", a.GetResource().Resource)
	}
}

func TestReadInventory_ListError(t *testing.T) {
	t.Parallel()

	cs := fake.NewClientset()
	cs.PrependReactor("list", "nodes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("connection refused")
	})
	md := metadatafake.NewSimpleMetadataClient(metadataScheme(t))

	r := inventoryAccessWith(t, cs, md)
	_, err := r.ReadInventory(t.Context(), uuid.New())
	require.ErrorContains(t, err, "list nodes: connection refused")
}
