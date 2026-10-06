package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ObjectStorageSpec defines an S3-compatible object store (Ceph RGW) on the
// shared Ceph cluster, using the disks that DiskPools contribute.
type ObjectStorageSpec struct {
	// Replicas is how many copies Ceph keeps of each piece of data, for both
	// the store's metadata and its contents. Absent means automatic: derived
	// from the number of nodes contributing disks (capped at 3).
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=3
	Replicas *int32 `json:"replicas,omitempty"`
	// GatewayInstances is the number of RGW gateway pods serving the S3 API.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=5
	// +kubebuilder:default=1
	GatewayInstances int32 `json:"gatewayInstances,omitempty"`
}

// ObjectStorage provides S3-compatible buckets over the shared Ceph cluster.
// It derives a StorageClass, named in status.storageClassName, for
// ObjectBucketClaims to reference; Rook provisions each claim's bucket and
// hands its credentials to the claim's Secret and ConfigMap.
//
// The name-length limit exists because Rook rejects CephObjectStore names
// over 38 (its rook-ceph-rgw-<store>-mime-types ConfigMap must fit in 63),
// and it validates deletes the same way, so an overlong store also wedges
// on removal. 38 minus the cephobj- prefix leaves 30.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="StorageClass",type=string,JSONPath=`.status.storageClassName`
// +kubebuilder:validation:XValidation:rule="size(self.metadata.name) <= 30",message="name must be at most 30 characters: Rook caps CephObjectStore names at 38 and the derived name is cephobj-<name>"
type ObjectStorage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +kubebuilder:default={}
	Spec   ObjectStorageSpec `json:"spec,omitempty"`
	Status ConsumerStatus    `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ObjectStorageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ObjectStorage `json:"items"`
}

func init() { SchemeBuilder.Register(&ObjectStorage{}, &ObjectStorageList{}) }
