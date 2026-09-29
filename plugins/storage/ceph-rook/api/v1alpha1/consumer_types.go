package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ReasonNoOSDs: no DiskPool contributes usable disks, so there is nothing
// to place data on. Consumer kinds (BlockStorage, FileStorage) share it.
const ReasonNoOSDs = "NoOSDs"

// ReasonRookFailure: the derived Rook object reports phase Failure, so the
// consumer cannot become Ready until Rook resolves it.
const ReasonRookFailure = "RookFailure"

// ConsumerStatus is the observed state shared by the consumer kinds
// (BlockStorage, FileStorage). Fields describe the derived StorageClass, sized
// against the whole shared Ceph cluster, not any single pool.
type ConsumerStatus struct {
	// Phase is Provisioning until the derived storage reports Ready, and
	// Degraded when reconciliation needs operator action.
	Phase string `json:"phase,omitempty"`
	// StorageClassName is the derived StorageClass's name — what a
	// PersistentVolumeClaim should reference.
	StorageClassName string `json:"storageClassName,omitempty"`
	// Replicas is the replica count that spec.replication resolved to.
	Replicas int `json:"replicas,omitempty"`
	// FailureDomain is where Ceph places the replicas of each piece of data:
	// "host" spreads them across nodes, "osd" only across disks.
	FailureDomain string `json:"failureDomain,omitempty"`
	// Message explains the current phase, naming the operator action needed
	// when the object is Degraded.
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

// ConsumerStatus and ReplicationSpec let one generic reconciler drive every
// consumer kind.

func (in *BlockStorage) ConsumerStatus() *ConsumerStatus { return &in.Status }
func (in *BlockStorage) ReplicationSpec() string         { return in.Spec.Replication }

func (in *FileStorage) ConsumerStatus() *ConsumerStatus { return &in.Status }
func (in *FileStorage) ReplicationSpec() string         { return in.Spec.Replication }
