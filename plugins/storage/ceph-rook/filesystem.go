package main

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// cephFSDataPoolName names the CephFilesystem's data pool. Rook derives the
// actual Ceph pool as <filesystem>-<this>, which the StorageClass's pool
// parameter must match exactly — both renderers share this constant so they
// cannot drift apart.
const cephFSDataPoolName = "data0"

// RenderCephFilesystem builds the CephFilesystem for a FileStorage. Both pools
// replicate at the same size: the metadata pool is small but every file op
// touches it, so it never deserves less redundancy than the data.
func RenderCephFilesystem(namespace, name string, replicas int, failureDomain string, activeMDS int64) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}

	u.SetAPIVersion(cephAPIVersion)
	u.SetKind("CephFilesystem")

	u.SetName(name)
	u.SetNamespace(namespace)

	dataPool := replicatedPoolSpec(replicas, failureDomain)
	dataPool["name"] = cephFSDataPoolName

	u.Object["spec"] = map[string]any{
		"metadataPool": replicatedPoolSpec(replicas, failureDomain),
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
