package shoot

import (
	"context"
	"hash/fnv"

	"github.com/google/uuid"
)

// Per-node capacity the mock reports, roughly a small general-purpose VM.
const (
	mockNodePods         = 110
	mockNodeCPUMilli     = 4000
	mockNodeMemoryMi     = 16384
	mockSystemPodsNode   = 8
	mockMaxNamespacePods = 12
)

// ReadInventory reports a plausible, deterministic inventory for local
// development (GARDENER_MODE=mock): every worker pool runs its autoscaler
// minimum of ready nodes, and each fundament namespace holds a pod count
// derived from its name, so the numbers stay put between collections.
func (m *MockShootAccess) ReadInventory(_ context.Context, clusterID uuid.UUID) (*Inventory, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.ReadInventoryError != nil {
		return nil, m.ReadInventoryError
	}

	inv := &Inventory{
		Pools:         make(map[string]PoolInventory),
		NamespacePods: make(map[uuid.UUID]int32),
	}

	var pools map[string]int32
	if m.WorkerPools != nil {
		pools = m.WorkerPools(clusterID)
	}
	for name, nodes := range pools {
		inv.Pools[name] = PoolInventory{Nodes: nodes, NodesReady: nodes}
		inv.Nodes += nodes
	}
	inv.NodesReady = inv.Nodes
	inv.PodAllocatable = inv.Nodes * mockNodePods
	inv.CPUAllocatableMilli = int64(inv.Nodes) * mockNodeCPUMilli
	inv.MemoryAllocatableMi = int64(inv.Nodes) * mockNodeMemoryMi
	inv.Pods = inv.Nodes * mockSystemPodsNode

	for name, ns := range m.Namespaces[clusterID] {
		id, err := uuid.Parse(ns.Labels[LabelNamespaceID])
		if err != nil {
			continue
		}
		pods := mockNamespacePods(name)
		inv.NamespacePods[id] = pods
		inv.Pods += pods
	}

	return inv, nil
}

// mockNamespacePods derives a pod count in [0, mockMaxNamespacePods] from a
// namespace name.
func mockNamespacePods(name string) int32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return int32(h.Sum32() % (mockMaxNamespacePods + 1)) //nolint:gosec // bounded by the modulus
}

var _ InventoryReader = (*MockShootAccess)(nil)
