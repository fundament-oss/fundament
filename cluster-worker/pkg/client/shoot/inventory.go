package shoot

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// LabelNamespaceID marks a namespace fundament manages with the id of its
	// tenant.namespaces row.
	LabelNamespaceID = "fundament.io/namespace-id"

	// LabelWorkerPool is the label Gardener's machine controller puts on every
	// node of a worker pool; its value is the worker name.
	LabelWorkerPool = "worker.gardener.cloud/pool"

	// activePodsSelector leaves out pods in a terminal phase: a finished Job's
	// pods linger until garbage-collected but use no capacity.
	activePodsSelector = "status.phase!=Succeeded,status.phase!=Failed"
)

// Inventory is what one collection pass reads from a shoot (FUN-23).
type Inventory struct {
	Nodes      int32
	NodesReady int32
	// Pods counts pods not in a terminal phase, in every namespace.
	Pods                int32
	PodAllocatable      int32
	CPUAllocatableMilli int64
	MemoryAllocatableMi int64
	// Pools counts nodes per Gardener worker pool, keyed by worker name. A node
	// without the pool label counts toward the totals only.
	Pools map[string]PoolInventory
	// NamespacePods counts active pods per fundament namespace, keyed by the
	// namespace id label. A namespace without pods is present with 0.
	NamespacePods map[uuid.UUID]int32
}

// PoolInventory is one worker pool's share of the nodes.
type PoolInventory struct {
	Nodes      int32
	NodesReady int32
}

// InventoryReader reads a shoot's nodes, pods and fundament namespaces.
type InventoryReader interface {
	ReadInventory(ctx context.Context, clusterID uuid.UUID) (*Inventory, error)
}

// ReadInventory lists nodes, fundament namespaces and the metadata of active
// pods. Every list asks for ResourceVersion "0", so the API server answers
// from its watch cache instead of a quorum read from etcd, and pods are listed
// as metadata only: the counts need neither specs nor statuses.
func (r *RealShootAccess) ReadInventory(ctx context.Context, clusterID uuid.UUID) (*Inventory, error) {
	cs, err := r.newClient(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	md, err := r.newMetadataClient(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{ResourceVersion: "0"})
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	namespaces, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector:   LabelNamespaceID,
		ResourceVersion: "0",
	})
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}
	pods, err := md.Resource(corev1.SchemeGroupVersion.WithResource("pods")).List(ctx, metav1.ListOptions{
		FieldSelector:   activePodsSelector,
		ResourceVersion: "0",
	})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}

	return FoldInventory(nodes.Items, namespaces.Items, pods.Items), nil
}

// FoldInventory reduces the listed objects to counts. Namespaces whose id
// label does not parse are skipped: they are not fundament's to report.
func FoldInventory(nodes []corev1.Node, namespaces []corev1.Namespace, pods []metav1.PartialObjectMetadata) *Inventory {
	inv := &Inventory{
		Pools:         make(map[string]PoolInventory),
		NamespacePods: make(map[uuid.UUID]int32),
	}

	for i := range nodes {
		node := &nodes[i]
		ready := nodeReady(node)

		inv.Nodes++
		if ready {
			inv.NodesReady++
		}
		if cpu, ok := node.Status.Allocatable[corev1.ResourceCPU]; ok {
			inv.CPUAllocatableMilli += cpu.MilliValue()
		}
		if mem, ok := node.Status.Allocatable[corev1.ResourceMemory]; ok {
			inv.MemoryAllocatableMi += mem.Value() >> 20
		}
		if podCap, ok := node.Status.Allocatable[corev1.ResourcePods]; ok {
			inv.PodAllocatable += int32(podCap.Value()) //nolint:gosec // a node allocates at most a few hundred pods
		}

		pool, ok := node.Labels[LabelWorkerPool]
		if !ok {
			continue
		}
		p := inv.Pools[pool]
		p.Nodes++
		if ready {
			p.NodesReady++
		}
		inv.Pools[pool] = p
	}

	namespaceIDs := make(map[string]uuid.UUID, len(namespaces))
	for i := range namespaces {
		id, err := uuid.Parse(namespaces[i].Labels[LabelNamespaceID])
		if err != nil {
			continue
		}
		namespaceIDs[namespaces[i].Name] = id
		inv.NamespacePods[id] = 0
	}

	for i := range pods {
		inv.Pods++
		if id, ok := namespaceIDs[pods[i].Namespace]; ok {
			inv.NamespacePods[id]++
		}
	}

	return inv
}

func nodeReady(node *corev1.Node) bool {
	for _, c := range node.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

var _ InventoryReader = (*RealShootAccess)(nil)
