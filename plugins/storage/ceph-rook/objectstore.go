package main

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// rgwPort is the in-cluster port of the RGW S3 service
// (rook-ceph-rgw-<store>). Plain HTTP: TLS is a gateway concern.
const rgwPort = 80

// RenderCephObjectStore builds the CephObjectStore for an ObjectStorage. Both
// pools replicate at the same size: the metadata pool is small but every S3
// op touches it, so it never deserves less redundancy than the data.
func RenderCephObjectStore(namespace, name string, replicas int, failureDomain string, gatewayInstances int64) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}

	u.SetAPIVersion(cephAPIVersion)
	u.SetKind("CephObjectStore")

	u.SetName(name)
	u.SetNamespace(namespace)

	u.Object["spec"] = map[string]any{
		"metadataPool": replicatedPoolSpec(replicas, failureDomain),
		"dataPool":     replicatedPoolSpec(replicas, failureDomain),
		// Deleting the CR (or its owner) must never destroy bucket data.
		"preservePoolsOnDelete": true,
		"gateway": map[string]any{
			"port":      int64(rgwPort),
			"instances": gatewayInstances,
		},
	}

	return u
}
