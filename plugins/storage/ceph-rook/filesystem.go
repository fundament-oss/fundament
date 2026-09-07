package main

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// RenderCephFilesystem builds the CephFilesystem for a FileStorage. Both pools
// replicate at the same size: the metadata pool is small but every file op
// touches it, so it never deserves less redundancy than the data.
func RenderCephFilesystem(namespace, name string, replicas int, failureDomain string, activeMDS int64) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}

	u.SetAPIVersion(cephAPIVersion)
	u.SetKind("CephFilesystem")

	u.SetName(name)
	u.SetNamespace(namespace)

	pool := func() map[string]any {
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

	// Named so the derived pool is <name>-data0, which the StorageClass's pool
	// parameter must match exactly.
	dataPool := pool()
	dataPool["name"] = "data0"

	u.Object["spec"] = map[string]any{
		"metadataPool": pool(),
		"dataPools":    []any{dataPool},
		// Deleting the CR (or its owner) must never destroy filesystem data.
		"preserveFilesystemOnDelete": true,
		"metadataServer": map[string]any{
			"activeCount":   activeMDS,
			"activeStandby": true,
		},
	}

	return u
}
