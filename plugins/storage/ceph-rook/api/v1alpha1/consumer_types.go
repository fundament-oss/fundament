package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ReasonNoOSDs: no DiskPool contributes usable disks, so there is nothing
// to place data on. All consumer kinds share it.
const ReasonNoOSDs = "NoOSDs"

// ReasonRookFailure: the derived Rook object reports phase Failure, so the
// consumer cannot become Ready until Rook resolves it.
const ReasonRookFailure = "RookFailure"

// ReasonDefaultConflict: more than one BlockStorage sets spec.default, so none
// of them marks its StorageClass as the cluster default.
const ReasonDefaultConflict = "DefaultConflict"

// ReasonDeletionBlocked: buckets still reference the ObjectStorage's derived
// StorageClass, which their claims' finalizers need to deprovision.
const ReasonDeletionBlocked = "DeletionBlocked"

// Detached from the doc comment below so it stays out of the CRDs: these
// docs publish into every consumer kind, so name no kind-specific field.

// ConsumerStatus is the observed state of a storage consumer and the
// StorageClass it derives.
type ConsumerStatus struct {
	// Phase is Provisioning until the derived storage reports Ready, and
	// Degraded when reconciliation needs operator action.
	Phase string `json:"phase,omitempty"`
	// StorageClassName is the name of the derived StorageClass, which claims
	// (PersistentVolumeClaims, or ObjectBucketClaims for an object store)
	// reference.
	StorageClassName string `json:"storageClassName,omitempty"`
	// Replicas is the replica count in effect: spec.replicas clamped to the
	// contributing node count, or the derived count when spec.replicas is absent.
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

// ConsumerStatus and RequestedReplicas let one generic reconciler drive every
// consumer kind.

func (in *BlockStorage) ConsumerStatus() *ConsumerStatus { return &in.Status }
func (in *BlockStorage) RequestedReplicas() *int32       { return in.Spec.Replicas }

func (in *FileStorage) ConsumerStatus() *ConsumerStatus { return &in.Status }
func (in *FileStorage) RequestedReplicas() *int32       { return in.Spec.Replicas }

func (in *ObjectStorage) ConsumerStatus() *ConsumerStatus { return &in.Status }
func (in *ObjectStorage) RequestedReplicas() *int32       { return in.Spec.Replicas }
