//go:generate controller-gen object paths=.
//go:generate controller-gen crd paths=. output:crd:dir=../../crds

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Phase is the high-level reconciliation state of a Peer.
type Phase string

const (
	// PhasePending means the peer's OpenFSC Manager/Controller are not yet ready.
	PhasePending Phase = "Pending"
	// PhaseActive means the peer's OpenFSC Manager/Controller are running.
	PhaseActive Phase = "Active"
	// PhaseError means reconciliation failed in a way that needs attention.
	PhaseError Phase = "Error"
)

// PeerSpec describes a member of an FSC group.
type PeerSpec struct {
	// GroupID is the FSC group this peer belongs to.
	GroupID string `json:"groupID"`
	// PeerID is this peer's identifier within the group (derived from the peer
	// certificate's subject serialNumber).
	PeerID string `json:"peerID"`
	// ManagerAddress is the https:// address of this peer's Manager.
	ManagerAddress string `json:"managerAddress,omitempty"`
	// Directory marks this peer as the group's Directory. The OpenFSC Manager
	// installed by this plugin functions as the Directory of its group.
	Directory bool `json:"directory,omitempty"`
}

// PeerStatus is the observed state of a Peer.
type PeerStatus struct {
	// Phase is the high-level reconciliation state.
	Phase Phase `json:"phase,omitempty"`
	// Message is a human-readable status detail.
	Message string `json:"message,omitempty"`
	// ControllerURL is the OpenFSC Controller UI address, set only on the
	// directory (self) peer so the console can link to it.
	ControllerURL string `json:"controllerURL,omitempty"`
	// ObservedGeneration is the .metadata.generation last reconciled.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// LastSyncedTime is when the peer was last reconciled.
	LastSyncedTime *metav1.Time `json:"lastSyncedTime,omitempty"`
	// Conditions follow the standard Kubernetes condition convention.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=peer
// +kubebuilder:printcolumn:name="Group",type=string,JSONPath=`.spec.groupID`
// +kubebuilder:printcolumn:name="Directory",type=boolean,JSONPath=`.spec.directory`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// Peer represents a member of an FSC group (cluster-scoped).
type Peer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PeerSpec   `json:"spec"`
	Status            PeerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PeerList is a list of Peer resources.
type PeerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Peer `json:"items"`
}

// Status is the common reconciliation status shared by the FSC gateway
// resources (Inway, Outway). Their reconcilers are observe-only: they reflect
// whether the declared gateway has registered with the OpenFSC Controller
// Administration API, since inways and outways self-register from their own
// workloads rather than being created by the operator.
type Status struct {
	// Phase is the high-level reconciliation state.
	Phase Phase `json:"phase,omitempty"`
	// Message is a human-readable status detail.
	Message string `json:"message,omitempty"`
	// URL is an in-cluster address for reaching this gateway, surfaced to the
	// console (e.g. the outway's consume endpoint). Empty when not applicable.
	URL string `json:"url,omitempty"`
	// ObservedGeneration is the .metadata.generation last reconciled.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// LastSyncedTime is when the resource was last reconciled to the Controller.
	LastSyncedTime *metav1.Time `json:"lastSyncedTime,omitempty"`
	// Conditions follow the standard Kubernetes condition convention.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// InwaySpec declares a provider gateway whose registration the operator tracks.
type InwaySpec struct {
	// InwayName is the FSC name the inway registers under with the Controller.
	InwayName string `json:"inwayName"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=inway
// +kubebuilder:printcolumn:name="Inway",type=string,JSONPath=`.spec.inwayName`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// Inway is a provider gateway through which services are published to the FSC
// group (cluster-scoped). The operator observes its registration; it does not
// create it.
type Inway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              InwaySpec `json:"spec"`
	Status            Status    `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// InwayList is a list of Inway resources.
type InwayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Inway `json:"items"`
}

// OutwaySpec declares a consumer gateway whose registration the operator tracks.
type OutwaySpec struct {
	// OutwayName is the FSC name the outway registers under with the Controller.
	OutwayName string `json:"outwayName"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=outway
// +kubebuilder:printcolumn:name="Outway",type=string,JSONPath=`.spec.outwayName`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// Outway is a consumer gateway through which services are consumed from the FSC
// group (cluster-scoped). The operator observes its registration; it does not
// create it.
type Outway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              OutwaySpec `json:"spec"`
	Status            Status     `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OutwayList is a list of Outway resources.
type OutwayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Outway `json:"items"`
}
