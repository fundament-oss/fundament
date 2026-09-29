package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// BlockStorageSpec defines a block-volume (ReadWriteOnce) StorageClass on the
// shared Ceph cluster, using the disks that DiskPools contribute.
type BlockStorageSpec struct {
	// Replication is how many copies Ceph keeps of each piece of data: an
	// explicit count, or "auto" to derive it from the number of nodes
	// contributing disks (capped at 3).
	// +kubebuilder:validation:Enum=auto;"1";"2";"3"
	// +kubebuilder:default=auto
	Replication string `json:"replication,omitempty"`
}

// BlockStorage provides block volumes (ReadWriteOnce) over the shared Ceph
// cluster: it derives a StorageClass, named in status.storageClassName, for
// PersistentVolumeClaims to reference.
//
// The name-length limit exists because the derived objects are named
// ceph-<name> and object names cap at 253 characters.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="StorageClass",type=string,JSONPath=`.status.storageClassName`
// +kubebuilder:validation:XValidation:rule="size(self.metadata.name) <= 248",message="name must be at most 248 characters: the derived ceph-<name> object name caps at 253"
type BlockStorage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BlockStorageSpec `json:"spec,omitempty"`
	Status            ConsumerStatus   `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type BlockStorageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BlockStorage `json:"items"`
}

func init() { SchemeBuilder.Register(&BlockStorage{}, &BlockStorageList{}) }
