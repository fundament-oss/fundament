package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// BlockStorageSpec asks for an RBD StorageClass over the shared OSD set.
// BlockStorage brings no disks; StoragePools do that.
type BlockStorageSpec struct {
	// Replication selects replica count; "auto" derives it from node count.
	// +kubebuilder:validation:Enum=auto;"1";"2";"3"
	// +kubebuilder:default=auto
	Replication string `json:"replication,omitempty"`
}

// ReasonNoOSDs: no StoragePool contributes usable disks, so there is nothing
// to place data on. Consumer kinds (BlockStorage, FileStorage) share it.
const ReasonNoOSDs = "NoOSDs"

// BlockStorageStatus is the observed state. Fields describe the derived
// CephBlockPool and StorageClass, sized against the whole cluster's OSD set.
type BlockStorageStatus struct {
	Phase            string `json:"phase,omitempty"`
	StorageClassName string `json:"storageClassName,omitempty"`
	Replicas         int    `json:"replicas,omitempty"`
	FailureDomain    string `json:"failureDomain,omitempty"`
	Message          string `json:"message,omitempty"`
	// ObservedGeneration is the metadata.generation this status was computed from.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Conditions carries ConditionReady. listType=map on type so the API server
	// merges by condition type rather than by position.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="StorageClass",type=string,JSONPath=`.status.storageClassName`
type BlockStorage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BlockStorageSpec   `json:"spec,omitempty"`
	Status            BlockStorageStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type BlockStorageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BlockStorage `json:"items"`
}

func init() { SchemeBuilder.Register(&BlockStorage{}, &BlockStorageList{}) }
