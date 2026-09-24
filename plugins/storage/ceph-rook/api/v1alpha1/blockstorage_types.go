package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// BlockStorageSpec asks for an RBD StorageClass over the shared OSD set.
// BlockStorage brings no disks; DiskPools do that.
type BlockStorageSpec struct {
	// Replication selects replica count; "auto" derives it from node count.
	// +kubebuilder:validation:Enum=auto;"1";"2";"3"
	// +kubebuilder:default=auto
	Replication string `json:"replication,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="StorageClass",type=string,JSONPath=`.status.storageClassName`
// The derived CephBlockPool/StorageClass is named ceph-<name>; object names cap
// at 253 characters, so a longer name would only fail at reconcile time,
// wedging the object Degraded on a create that can never succeed.
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
