package main

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// RenderCephBlockPool creates an unstructured Kubernetes object for a Rook Ceph block pool.
// It sets the replica size and failure domain for the pool configuration.
func RenderCephBlockPool(namespace, name string, replicas int, failureDomain string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}

	// Set API version and kind
	u.SetAPIVersion("ceph.rook.io/v1")
	u.SetKind("CephBlockPool")

	// Set metadata
	u.SetName(name)
	u.SetNamespace(namespace)

	// Set spec fields
	spec := make(map[string]any)
	spec["failureDomain"] = failureDomain

	// Set replicated size (must be int64)
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
