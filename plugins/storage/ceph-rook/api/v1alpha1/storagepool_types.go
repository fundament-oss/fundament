package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// StoragePoolSpec is the operator's desired block storage.
type StoragePoolSpec struct {
	// Disks are the names of Disk objects to consume as OSDs. listType=set so the
	// API server rejects a repeat, which would be double-counted in status.
	// +optional
	// +listType=set
	Disks []string `json:"disks,omitempty"`
	// Replication selects replica count; "auto" derives it from node count.
	// +kubebuilder:validation:Enum=auto;"1";"2";"3"
	// +kubebuilder:default=auto
	Replication string `json:"replication,omitempty"`
}

// Pool phases. Provisioning and Ready track the backing CephBlockPool; Degraded
// means the pool needs operator action.
const (
	PhaseProvisioning = "Provisioning"
	PhaseReady        = "Ready"
	PhaseDegraded     = "Degraded"
)

// ConditionReady is the one condition every StoragePool carries. Paired with
// status.observedGeneration it is what `kubectl wait --for=condition=Ready` and
// a Flux/Argo health check read; phase alone cannot say whether the controller
// has seen the current spec.
const ConditionReady = "Ready"

// Reasons for ConditionReady. Kubernetes requires a reason on every condition,
// and it is the machine-readable half: message is prose, reason is matchable.
const (
	// ReasonReady: the derived CephBlockPool reports Ready.
	ReasonReady = "Ready"
	// ReasonProvisioning: the CephBlockPool exists (or is being created) but has
	// not reported Ready yet.
	ReasonProvisioning = "Provisioning"
	// ReasonNoUsableDisks: spec.disks resolved to nothing, so there is no OSD to
	// build a pool on.
	ReasonNoUsableDisks = "NoUsableDisks"
	// ReasonReconcileError: the reconcile itself failed; message carries the error.
	ReasonReconcileError = "ReconcileError"
)

// StoragePoolStatus is the observed state.
//
// Every field describes this pool's contribution to one shared Ceph cluster, not
// storage that belongs to it: all pools feed a single OSD set, and the derived
// CephBlockPool has no CRUSH rule confining it to spec.disks.
type StoragePoolStatus struct {
	Phase            string `json:"phase,omitempty"`
	StorageClassName string `json:"storageClassName,omitempty"`
	// Sized against the nodes contributing disks to the whole cluster, since that
	// is what bounds where Ceph can place a replica.
	Replicas      int    `json:"replicas,omitempty"`
	FailureDomain string `json:"failureDomain,omitempty"`
	// SelectedDiskCount is how many of spec.disks resolved to a usable Disk, not
	// how many OSDs are running: Rook creates those asynchronously, and removing a
	// disk from spec never removes its OSD (that needs a Ceph purge).
	SelectedDiskCount int `json:"selectedDiskCount,omitempty"`
	// RawCapacityBytes is the summed size of the disks this pool contributes,
	// before replication. Not the pool's capacity: volumes draw on the whole
	// cluster's OSDs, so dividing by Replicas means nothing. Use `ceph df`.
	RawCapacityBytes int64  `json:"rawCapacityBytes,omitempty"`
	Message          string `json:"message,omitempty"`
	// ObservedGeneration is the metadata.generation this status was computed
	// from. Below metadata.generation means the controller has not caught up with
	// the current spec, and every field above describes an older one.
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
