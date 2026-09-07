package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// FileStorageSpec asks for a shared (ReadWriteMany) CephFS StorageClass over
// the shared OSD set. FileStorage brings no disks; StoragePools do that.
type FileStorageSpec struct {
	// Replication selects replica count for both the metadata and data pool;
	// "auto" derives it from node count.
	// +kubebuilder:validation:Enum=auto;"1";"2";"3"
	// +kubebuilder:default=auto
	Replication string `json:"replication,omitempty"`
	// MetadataServers is the number of active MDS daemons. Each active gets a
	// standby (activeStandby). More than 1 only helps metadata-heavy workloads
	// at scale.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=5
	// +kubebuilder:default=1
	MetadataServers int32 `json:"metadataServers,omitempty"`
}

// FileStorageStatus is the observed state; same contract as BlockStorageStatus.
type FileStorageStatus struct {
	Phase            string `json:"phase,omitempty"`
	StorageClassName string `json:"storageClassName,omitempty"`
	Replicas         int    `json:"replicas,omitempty"`
	FailureDomain    string `json:"failureDomain,omitempty"`
	Message          string `json:"message,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
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
type FileStorage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FileStorageSpec   `json:"spec,omitempty"`
	Status            FileStorageStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type FileStorageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FileStorage `json:"items"`
}

func init() { SchemeBuilder.Register(&FileStorage{}, &FileStorageList{}) }
