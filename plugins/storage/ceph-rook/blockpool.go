package main

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// RenderCephBlockPool builds the CephBlockPool for a StoragePool.
func RenderCephBlockPool(namespace, name string, replicas int, failureDomain string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}

	u.SetAPIVersion("ceph.rook.io/v1")
	u.SetKind("CephBlockPool")

	u.SetName(name)
	u.SetNamespace(namespace)

	spec := make(map[string]any)
	spec["failureDomain"] = failureDomain

	replicated := make(map[string]any)
	replicated["size"] = int64(replicas)
	// Ceph refuses a size-1 pool unless the safety check is waived.
	if replicas < 2 {
		replicated["requireSafeReplicaSize"] = false
	}
	spec["replicated"] = replicated

	u.Object["spec"] = spec

	return u
}
