package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ReasonNoOSDs: no StoragePool contributes usable disks, so there is nothing
// to place data on. Consumer kinds (BlockStorage, FileStorage) share it.
const ReasonNoOSDs = "NoOSDs"

// ReasonRookFailure: the derived Rook object reports phase Failure, so the
// consumer cannot become Ready until Rook resolves it.
const ReasonRookFailure = "RookFailure"

// ConsumerStatus is the observed state shared by the consumer kinds
// (BlockStorage, FileStorage). Fields describe the derived Rook object and
// StorageClass, sized against the whole cluster's OSD set.
type ConsumerStatus struct {
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

// ConsumerStatus and ReplicationSpec let one generic reconciler drive every
// consumer kind.

func (in *BlockStorage) ConsumerStatus() *ConsumerStatus { return &in.Status }
func (in *BlockStorage) ReplicationSpec() string         { return in.Spec.Replication }

func (in *FileStorage) ConsumerStatus() *ConsumerStatus { return &in.Status }
func (in *FileStorage) ReplicationSpec() string         { return in.Spec.Replication }
