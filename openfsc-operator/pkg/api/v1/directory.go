package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Condition types reported on a Directory.
const (
	// ConditionReady is True once the directory's OpenFSC Manager and Controller
	// are Available.
	ConditionReady = "Ready"
	// ConditionPrerequisitesMet is True when the CRDs the operator depends on
	// (cert-manager, CloudNativePG) are present in the cluster.
	ConditionPrerequisitesMet = "PrerequisitesMet"
	// ConditionDeployed is True once the directory's prerequisite resources and
	// the OpenFSC umbrella release have been applied for the current generation.
	ConditionDeployed = "Deployed"
)

// DirectoryPostgres configures the CloudNativePG cluster the operator
// provisions for the directory's OpenFSC components.
type DirectoryPostgres struct {
	// Instances is the number of PostgreSQL instances.
	// +kubebuilder:default=1
	// +optional
	Instances int32 `json:"instances,omitempty"`
	// Image is the PostgreSQL container image.
	// +kubebuilder:default="ghcr.io/cloudnative-pg/postgresql:16"
	// +optional
	Image string `json:"image,omitempty"`
	// StorageClass is the StorageClass for the data volumes.
	// +kubebuilder:default="basic-csi"
	// +optional
	StorageClass string `json:"storageClass,omitempty"`
	// StorageSize is the size of each data volume.
	// +kubebuilder:default="1Gi"
	// +optional
	StorageSize string `json:"storageSize,omitempty"`
}

// DirectorySpec declares a self-contained OpenFSC directory peer: the operator
// installs the OpenFSC umbrella (Manager + Controller + auditlog + txlog-api),
// a self-signed group CA with the Manager's group certificate, and a
// CloudNativePG cluster backing the components.
type DirectorySpec struct {
	// GroupID is the FSC group this directory serves.
	// +kubebuilder:default="fsc-demo"
	// +optional
	GroupID string `json:"groupID,omitempty"`
	// PeerID is the directory peer's identifier within the group, carried as the
	// subject serialNumber on the Manager's group certificate.
	// +kubebuilder:default="12345678901234567899"
	// +optional
	PeerID string `json:"peerID,omitempty"`
	// Namespace is where the OpenFSC components are installed.
	// +kubebuilder:default="fsc"
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// ControllerURL is the host-reachable URL of the Controller UI, surfaced in
	// the Directory and self-Peer status so a console can link to it. Empty hides
	// the link.
	// +optional
	ControllerURL string `json:"controllerURL,omitempty"`
	// Postgres configures the CloudNativePG cluster backing the components.
	// The default {} makes the nested field defaults apply when omitted.
	// +kubebuilder:default={}
	// +optional
	Postgres DirectoryPostgres `json:"postgres,omitempty"`
	// AutoSignGrants lists the grant types the Manager signs automatically.
	// +kubebuilder:default={"servicePublication","delegatedServicePublication"}
	// +optional
	AutoSignGrants []string `json:"autoSignGrants,omitempty"`
}

// DirectoryStatus is the observed state of a Directory.
type DirectoryStatus struct {
	// Phase is the high-level reconciliation state.
	Phase Phase `json:"phase,omitempty"`
	// Message is a human-readable status detail.
	Message string `json:"message,omitempty"`
	// ControllerURL is the OpenFSC Controller UI address (copied from the spec
	// once the directory is deployed).
	ControllerURL string `json:"controllerURL,omitempty"`
	// ObservedGeneration is the .metadata.generation last reconciled.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Conditions follow the standard Kubernetes condition convention. The
	// operator reports Ready and PrerequisitesMet.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=directory
// +kubebuilder:printcolumn:name="Group",type=string,JSONPath=`.spec.groupID`
// +kubebuilder:printcolumn:name="Namespace",type=string,JSONPath=`.spec.namespace`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// Directory deploys a self-contained OpenFSC directory peer (cluster-scoped).
// The operator installs the OpenFSC core into spec.namespace and seeds the
// "self" Peer representing it.
type Directory struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DirectorySpec   `json:"spec"`
	Status            DirectoryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DirectoryList is a list of Directory resources.
type DirectoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Directory `json:"items"`
}
