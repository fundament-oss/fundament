package main

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// replicatedPoolSpec builds the replicated pool spec shared by CephBlockPool
// and both CephFilesystem pools, so a replication-policy change cannot land in
// one renderer and miss the other.
func replicatedPoolSpec(replicas int, failureDomain string) map[string]any {
	replicated := map[string]any{"size": int64(replicas)}
	// Ceph refuses a size-1 pool unless the safety check is waived.
	if replicas < 2 {
		replicated["requireSafeReplicaSize"] = false
	}
	return map[string]any{
		"failureDomain": failureDomain,
		"replicated":    replicated,
	}
}

// RenderCephBlockPool builds the CephBlockPool for a DiskPool.
func RenderCephBlockPool(namespace, name string, replicas int, failureDomain string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}

	u.SetAPIVersion(cephAPIVersion)
	u.SetKind("CephBlockPool")

	u.SetName(name)
	u.SetNamespace(namespace)

	u.Object["spec"] = replicatedPoolSpec(replicas, failureDomain)

	return u
}
