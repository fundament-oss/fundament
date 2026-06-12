//go:generate controller-gen object paths=.
//go:generate controller-gen crd paths=. output:crd:dir=../../chart/crds

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Phase is the high-level reconciliation state of an FSCInstallation.
type Phase string

const (
	// PhasePending means the installation's components are not all ready yet.
	PhasePending Phase = "Pending"
	// PhaseActive means the OpenFSC core is running and every declared gateway
	// has registered with the Controller.
	PhaseActive Phase = "Active"
	// PhaseError means reconciliation failed in a way that needs attention.
	PhaseError Phase = "Error"
)

// Condition types reported on an FSCInstallation.
const (
	// ConditionPrerequisitesMet is True when the CRDs the operator depends on
	// (cert-manager, CloudNativePG) are present in the cluster.
	ConditionPrerequisitesMet = "PrerequisitesMet"
	// ConditionCertificatesReady is True once the installation's group
	// certificates exist (minted in Self mode, referenced in External mode).
	ConditionCertificatesReady = "CertificatesReady"
	// ConditionCoreDeployed is True once the prerequisite resources and the
	// OpenFSC umbrella release have been applied for the current generation.
	ConditionCoreDeployed = "CoreDeployed"
	// ConditionReady is True once the OpenFSC Manager and Controller are
	// Available and all declared gateways are registered.
	ConditionReady = "Ready"
)

// DirectoryMode selects where the installation's FSC group Directory lives.
// +kubebuilder:validation:Enum=Self;External
type DirectoryMode string

const (
	// DirectoryModeSelf runs this installation's Manager as the group's
	// Directory, with a self-signed group CA. This makes the installation a
	// self-contained group, which is sufficient for local development; other
	// installations can join it via External mode.
	DirectoryModeSelf DirectoryMode = "Self"
	// DirectoryModeExternal joins an existing group through its Directory.
	DirectoryModeExternal DirectoryMode = "External"
)

// DirectoryConfig declares the installation's FSC group Directory.
// +kubebuilder:validation:XValidation:rule="(self.mode == 'External') == has(self.external)",message="external is required for mode External and forbidden for mode Self"
type DirectoryConfig struct {
	Mode DirectoryMode `json:"mode"`
	// +optional
	External *ExternalDirectory `json:"external,omitempty"`
}

// ExternalDirectory points at the existing group Directory to join.
type ExternalDirectory struct {
	// Address is the https:// address of the Directory peer's Manager.
	// +kubebuilder:validation:Pattern=`^https://.+`
	Address string `json:"address"`
	// PeerID is the Directory peer's identifier within the group.
	// +kubebuilder:validation:MinLength=1
	PeerID string `json:"peerID"`
	// TrustAnchor references the group CA certificate PEM all peers of the
	// group trust.
	TrustAnchor SecretKeySelector `json:"trustAnchor"`
}

// SecretKeySelector references one key of a Secret in the installation's
// namespace.
type SecretKeySelector struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +kubebuilder:default="ca.crt"
	// +optional
	Key string `json:"key,omitempty"`
}

// CertificateRef references a kubernetes.io/tls Secret in the installation's
// namespace carrying a group certificate (tls.crt/tls.key) issued by the
// group's CA.
type CertificateRef struct {
	// +kubebuilder:validation:MinLength=1
	ExistingSecret string `json:"existingSecret"`
}

// PostgresConfig sizes the CloudNativePG cluster the operator provisions for
// the installation's OpenFSC components.
type PostgresConfig struct {
	// Instances is the number of PostgreSQL instances.
	// +kubebuilder:default=1
	// +optional
	Instances int32 `json:"instances,omitempty"`
	// Image is the PostgreSQL container image.
	// +kubebuilder:default="ghcr.io/cloudnative-pg/postgresql:16"
	// +optional
	Image string `json:"image,omitempty"`
	// StorageClass is the StorageClass for the data volumes. There is no
	// default: it depends on the cluster (e.g. local-path on k3s).
	// +kubebuilder:validation:MinLength=1
	StorageClass string `json:"storageClass"`
	// StorageSize is the size of each data volume.
	// +kubebuilder:default="1Gi"
	// +optional
	StorageSize string `json:"storageSize,omitempty"`
}

// InwayConfig declares a provider gateway: services are published to the
// group through it.
type InwayConfig struct {
	// Name is the FSC name the inway registers under with the Controller, and
	// the suffix of its Helm release.
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	// +kubebuilder:validation:MaxLength=30
	Name string `json:"name"`
	// SelfAddress is the https:// address other peers use to reach this inway.
	// Defaults to the in-cluster service URL, which only peers in the same
	// cluster can reach; set it when the group has peers elsewhere.
	// +kubebuilder:validation:Pattern=`^https://.+`
	// +optional
	SelfAddress string `json:"selfAddress,omitempty"`
	// Certificate overrides the group certificate this inway presents
	// (defaults to spec.certificate). Only valid in External mode; Self mode
	// mints per-gateway certificates.
	// +optional
	Certificate *CertificateRef `json:"certificate,omitempty"`
}

// OutwayConfig declares a consumer gateway: services of the group are
// consumed through it.
type OutwayConfig struct {
	// Name is the FSC name the outway registers under with the Controller, and
	// the suffix of its Helm release.
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	// +kubebuilder:validation:MaxLength=30
	Name string `json:"name"`
	// Certificate overrides the group certificate this outway presents
	// (defaults to spec.certificate). Only valid in External mode; Self mode
	// mints per-gateway certificates.
	// +optional
	Certificate *CertificateRef `json:"certificate,omitempty"`
}

// FSCInstallationSpec declares one OpenFSC installation: the operator installs
// the OpenFSC core (Manager, Controller, audit/transaction logs, backed by a
// CloudNativePG cluster) into the resource's namespace and one gateway
// workload per declared inway/outway.
// +kubebuilder:validation:XValidation:rule="(self.directory.mode == 'External') == has(self.certificate)",message="certificate is required for directory.mode External and forbidden for Self"
// +kubebuilder:validation:XValidation:rule="self.directory.mode == 'External' || !has(self.inways) || !self.inways.exists(i, has(i.certificate))",message="inway certificate overrides are only valid for directory.mode External"
// +kubebuilder:validation:XValidation:rule="self.directory.mode == 'External' || !has(self.outways) || !self.outways.exists(o, has(o.certificate))",message="outway certificate overrides are only valid for directory.mode External"
type FSCInstallationSpec struct {
	// GroupID is the FSC group this installation belongs to.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="groupID is immutable"
	GroupID string `json:"groupID"`
	// PeerID is this installation's identity within the group, carried as the
	// subject serialNumber on its group certificates.
	// +kubebuilder:validation:MinLength=1
	PeerID    string          `json:"peerID"`
	Directory DirectoryConfig `json:"directory"`
	// Certificate is the group certificate the Manager presents as its peer,
	// token and signature identity. Required in External mode; in Self mode
	// the operator mints it from the self-signed group CA.
	// +optional
	Certificate *CertificateRef `json:"certificate,omitempty"`
	// ManagerAddress is the https:// address other peers use to reach this
	// installation's Manager. Defaults to the in-cluster service URL, which
	// only peers in the same cluster can reach; set it when the group has
	// peers elsewhere.
	// +kubebuilder:validation:Pattern=`^https://.+`
	// +optional
	ManagerAddress string         `json:"managerAddress,omitempty"`
	Postgres       PostgresConfig `json:"postgres"`
	// ControllerURL is the host-reachable URL of the Controller UI, surfaced
	// in the status so a console can link to it. Empty hides the link.
	// +kubebuilder:validation:Pattern=`^https?://.+`
	// +optional
	ControllerURL string `json:"controllerURL,omitempty"`
	// AutoSignGrants lists the grant types the Manager signs automatically.
	// +kubebuilder:default={"servicePublication","delegatedServicePublication"}
	// +optional
	AutoSignGrants []string `json:"autoSignGrants,omitempty"`
	// +kubebuilder:validation:MaxItems=20
	// +listType=map
	// +listMapKey=name
	// +optional
	Inways []InwayConfig `json:"inways,omitempty"`
	// +kubebuilder:validation:MaxItems=20
	// +listType=map
	// +listMapKey=name
	// +optional
	Outways []OutwayConfig `json:"outways,omitempty"`
}

// GatewayStatus is the observed state of one declared inway or outway.
type GatewayStatus struct {
	Name string `json:"name"`
	// Phase is Active once the gateway has registered with the Controller.
	Phase Phase `json:"phase,omitempty"`
	// Message is a human-readable status detail.
	Message string `json:"message,omitempty"`
	// URL is an address for reaching this gateway (the inway's self address,
	// the outway's in-cluster consume endpoint). Empty when not applicable.
	URL string `json:"url,omitempty"`
	// LastSyncedTime is when the gateway's registration was last confirmed.
	LastSyncedTime *metav1.Time `json:"lastSyncedTime,omitempty"`
}

// FSCInstallationStatus is the observed state of an FSCInstallation.
type FSCInstallationStatus struct {
	// Phase is the high-level reconciliation state.
	Phase Phase `json:"phase,omitempty"`
	// Message is a human-readable status detail.
	Message string `json:"message,omitempty"`
	// ManagerAddress is the address other peers use to reach this Manager;
	// an External-mode installation elsewhere joins this installation's group
	// by pointing its directory at this address.
	ManagerAddress string `json:"managerAddress,omitempty"`
	// ControllerURL is the OpenFSC Controller UI address (copied from the spec
	// once the core is deployed).
	ControllerURL string `json:"controllerURL,omitempty"`
	// ObservedGeneration is the .metadata.generation last reconciled.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Conditions follow the standard Kubernetes condition convention.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// +listType=map
	// +listMapKey=name
	// +optional
	Inways []GatewayStatus `json:"inways,omitempty"`
	// +listType=map
	// +listMapKey=name
	// +optional
	Outways []GatewayStatus `json:"outways,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=fsci
// +kubebuilder:printcolumn:name="Group",type=string,JSONPath=`.spec.groupID`
// +kubebuilder:printcolumn:name="Directory",type=string,JSONPath=`.spec.directory.mode`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// FSCInstallation makes its namespace a peer in an FSC group. The operator
// supports one FSCInstallation per namespace; all component and release names
// are fixed, so every installation's namespace looks the same.
type FSCInstallation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FSCInstallationSpec   `json:"spec"`
	Status            FSCInstallationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FSCInstallationList is a list of FSCInstallation resources.
type FSCInstallationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FSCInstallation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FSCInstallation{}, &FSCInstallationList{})
}
