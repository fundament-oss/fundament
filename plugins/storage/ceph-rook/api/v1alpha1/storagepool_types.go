package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// StoragePoolSpec is the operator's desired block storage.
type StoragePoolSpec struct {
	// Disks are the names of Disk objects to consume as OSDs.
	// +optional
	Disks []string `json:"disks,omitempty"`
	// Replication selects replica count; "auto" derives it from node count.
	// +kubebuilder:validation:Enum=auto;"1";"2";"3"
	// +kubebuilder:default=auto
	Replication string `json:"replication,omitempty"`
}

// Pool phases. Provisioning and Ready track the backing CephBlockPool; Degraded
// means this pool cannot be reconciled without operator action (a disk claimed
// by another pool, or a derived object owned by someone else).
const (
	PhaseProvisioning = "Provisioning"
	PhaseReady        = "Ready"
	PhaseDegraded     = "Degraded"
)

// StoragePoolStatus is the observed state.
type StoragePoolStatus struct {
	Phase            string `json:"phase,omitempty"`
	StorageClassName string `json:"storageClassName,omitempty"`
	Replicas         int    `json:"replicas,omitempty"`
	FailureDomain    string `json:"failureDomain,omitempty"`
	// SelectedDiskCount is how many of spec.disks resolved to a usable Disk.
	// It is not the number of OSDs Ceph currently has running: Rook creates
	// those asynchronously, and removing a disk from spec never removes its
	// OSD (that needs a Ceph purge).
	SelectedDiskCount int `json:"selectedDiskCount,omitempty"`
	// RawCapacityBytes is the summed size of the selected disks, before
	// replication. Usable capacity is roughly this divided by replicas.
	RawCapacityBytes int64  `json:"rawCapacityBytes,omitempty"`
	Message          string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="StorageClass",type=string,JSONPath=`.status.storageClassName`
type StoragePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              StoragePoolSpec   `json:"spec,omitempty"`
	Status            StoragePoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type StoragePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StoragePool `json:"items"`
}

func init() { SchemeBuilder.Register(&StoragePool{}, &StoragePoolList{}) }
