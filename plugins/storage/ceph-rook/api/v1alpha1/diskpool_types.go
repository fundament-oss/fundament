package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DiskPoolSpec selects the disks this pool contributes to the shared Ceph
// cluster.
type DiskPoolSpec struct {
	// Disks are the Disk objects to contribute. Ceph runs one storage daemon
	// (OSD) per disk. Each name may appear once.
	// +optional
	// +listType=map
	// +listMapKey=name
	Disks []PoolDisk `json:"disks,omitempty"`
}

// PoolDisk selects one Disk for a DiskPool, in the shape of a Rook
// storage.nodes[].devices[] entry.
type PoolDisk struct {
	// Name is the name of the Disk object.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// Pool phases. PhaseProvisioning is used by consumer kinds only; DiskPool
// itself is Ready or Degraded.
const (
	PhaseProvisioning = "Provisioning"
	PhaseReady        = "Ready"
	PhaseDegraded     = "Degraded"
)

// ConditionReady is the one condition every DiskPool carries. Paired with
// status.observedGeneration it is what `kubectl wait --for=condition=Ready` and
// a Flux/Argo health check read; phase alone cannot say whether the controller
// has seen the current spec.
const ConditionReady = "Ready"

// Reasons for ConditionReady. Kubernetes requires a reason on every condition,
// and it is the machine-readable half: message is prose, reason is matchable.
const (
	// ReasonReady: the derived Rook object is Ready (DiskPool: the pool's
	// disk contribution is in order).
	ReasonReady = "Ready"
	// ReasonProvisioning: the derived object exists (or is being created) but
	// has not reported Ready yet.
	ReasonProvisioning = "Provisioning"
	// ReasonNoUsableDisks: spec.disks resolved to nothing, so there is no OSD to
	// build a pool on.
	ReasonNoUsableDisks = "NoUsableDisks"
	// ReasonCephClusterMissing: the singleton CephCluster does not exist, so the
	// pool's disks are not recorded anywhere yet.
	ReasonCephClusterMissing = "CephClusterMissing"
	// ReasonReconcileError: the reconcile itself failed; message carries the error.
	ReasonReconcileError = "ReconcileError"
)

// DiskPoolStatus is the observed state of this pool's contribution to the
// shared Ceph cluster. Volumes are placed across the disks of all pools.
type DiskPoolStatus struct {
	Phase string `json:"phase,omitempty"`
	// SelectedDiskCount is how many of spec.disks resolved to a usable Disk. Ceph
	// creates their storage daemons (OSDs) asynchronously, and removing a disk
	// from spec keeps its OSD until a manual Ceph purge.
	SelectedDiskCount int `json:"selectedDiskCount,omitempty"`
	// RawCapacity is the summed size of the disks this pool contributes,
	// before replication. Volumes are placed across every disk in the shared
	// cluster; `ceph df` shows free space.
	RawCapacity resource.Quantity `json:"rawCapacity,omitempty"`
	// Message explains the current phase, naming the operator action needed
	// when the pool is Degraded.
	Message string `json:"message,omitempty"`
	// ObservedGeneration is the metadata.generation this status was computed from.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Conditions carries the Ready condition, keyed by condition type.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// DiskPool contributes a set of discovered disks to the shared Ceph cluster.
// A BlockStorage or FileStorage turns that capacity into a StorageClass.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
type DiskPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DiskPoolSpec   `json:"spec,omitempty"`
	Status            DiskPoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type DiskPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DiskPool `json:"items"`
}

func init() { SchemeBuilder.Register(&DiskPool{}, &DiskPoolList{}) }
