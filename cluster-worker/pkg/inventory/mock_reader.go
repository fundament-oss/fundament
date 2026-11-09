package inventory

import (
	"context"
	"fmt"
	"hash/fnv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/common/kubename"
)

// Per-node capacity the mock reports, roughly a small general-purpose VM.
const (
	mockNodePods         = 110
	mockNodeCPUMilli     = 4000
	mockNodeMemoryMi     = 16384
	mockSystemPodsNode   = 8
	mockMaxNamespacePods = 12
)

// MockReader reports a plausible, deterministic inventory for local
// development (GARDENER_MODE=mock): every live node pool runs its autoscaler
// minimum of ready nodes, and each live namespace holds a pod count derived
// from its name, so the numbers stay put between collections. It reads the
// database rather than the mock Gardener client, whose shoots live in the
// memory of the provisioning worker, not of this one.
type MockReader struct {
	queries *db.Queries
}

// NewMockReader creates a mock inventory reader over the database.
func NewMockReader(pool *pgxpool.Pool) *MockReader {
	return &MockReader{queries: db.New(pool)}
}

var _ shoot.InventoryReader = (*MockReader)(nil)

func (r *MockReader) ReadInventory(ctx context.Context, clusterID uuid.UUID) (*shoot.Inventory, error) {
	pools, err := r.queries.InventoryMockNodePoolList(ctx, db.InventoryMockNodePoolListParams{ClusterID: clusterID})
	if err != nil {
		return nil, fmt.Errorf("list node pools: %w", err)
	}
	namespaces, err := r.queries.InventoryMockNamespaceList(ctx, db.InventoryMockNamespaceListParams{ClusterID: clusterID})
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}

	inv := &shoot.Inventory{
		Pools:         make(map[string]shoot.PoolInventory, len(pools)),
		NamespacePods: make(map[uuid.UUID]int32, len(namespaces)),
	}
	for _, p := range pools {
		inv.Pools[kubename.GenerateWorkerName(p.Name)] = shoot.PoolInventory{Nodes: p.AutoscaleMin, NodesReady: p.AutoscaleMin}
		inv.Nodes += p.AutoscaleMin
	}
	inv.NodesReady = inv.Nodes
	inv.PodAllocatable = inv.Nodes * mockNodePods
	inv.CPUAllocatableMilli = int64(inv.Nodes) * mockNodeCPUMilli
	inv.MemoryAllocatableMi = int64(inv.Nodes) * mockNodeMemoryMi
	inv.Pods = inv.Nodes * mockSystemPodsNode

	for _, ns := range namespaces {
		pods := mockNamespacePods(ns.Name)
		inv.NamespacePods[ns.ID] = pods
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
