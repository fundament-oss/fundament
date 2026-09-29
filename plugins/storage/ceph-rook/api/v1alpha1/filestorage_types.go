package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// FileStorageSpec defines a shared-filesystem (ReadWriteMany) StorageClass on
// the shared Ceph cluster, using the disks that DiskPools contribute.
type FileStorageSpec struct {
	// Replication is how many copies Ceph keeps of each piece of data, for both
	// the filesystem's metadata and its contents: an explicit count, or "auto"
	// to derive it from the number of nodes contributing disks (capped at 3).
	// +kubebuilder:validation:Enum=auto;"1";"2";"3"
	// +kubebuilder:default=auto
	Replication string `json:"replication,omitempty"`
	// MetadataServers is the number of active filesystem metadata servers. Each
	// active server gets its own standby for failover.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=5
	// +kubebuilder:default=1
	MetadataServers int32 `json:"metadataServers,omitempty"`
}

// FileStorage provides shared volumes (ReadWriteMany), which many pods on many
// nodes can mount at once, over the shared Ceph cluster. It derives a
// StorageClass, named in status.storageClassName, for PersistentVolumeClaims
// to reference.
//
// The name-length limit exists because Rook labels the metadata-server
// deployment rook_file_system=cephfs-<name> and label values cap at 63
// characters.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="StorageClass",type=string,JSONPath=`.status.storageClassName`
// +kubebuilder:validation:XValidation:rule="size(self.metadata.name) <= 56",message="name must be at most 56 characters: Rook derives a cephfs-<name> label capped at 63"
type FileStorage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +kubebuilder:default={}
	Spec   FileStorageSpec `json:"spec,omitempty"`
	Status ConsumerStatus  `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type FileStorageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FileStorage `json:"items"`
}

func init() { SchemeBuilder.Register(&FileStorage{}, &FileStorageList{}) }
