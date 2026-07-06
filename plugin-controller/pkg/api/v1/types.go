//go:generate controller-gen object paths=.

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PluginPhase represents the phase of a plugin installation.
type PluginPhase string

const (
	PluginPhasePending     PluginPhase = "Pending"
	PluginPhaseDeploying   PluginPhase = "Deploying"
	PluginPhaseRunning     PluginPhase = "Running"
	PluginPhaseDegraded    PluginPhase = "Degraded"
	PluginPhaseFailed      PluginPhase = "Failed"
	PluginPhaseTerminating PluginPhase = "Terminating"
)

// PluginInstallation represents an installed plugin.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PluginInstallation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PluginInstallationSpec   `json:"spec"`
	Status            PluginInstallationStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen=true
type PluginInstallationSpec struct {
	Image           string            `json:"image"`
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// DefinitionRef is the immutable pin to the published PluginDefinition the
	// installer consented to. plugin-controller resolves the definition by
	// DefinitionHash and materialises the plugin SA's Role from it (FUN-17).
	// It is the source of truth for the plugin's RBAC scope — no RBAC is
	// copied onto this CR.
	//
	// The installation's addressable handle is metadata.name (Kubernetes
	// convention — no spec.pluginName echo of it). The controller uses
	// metadata.name to derive child resource names (namespace, SA, etc.);
	// DefinitionRef.PluginName names the pinned definition and may differ.
	DefinitionRef DefinitionRef `json:"definitionRef"`
	// ClusterRoles is legacy: once the controller materialises the SA Role
	// from DefinitionRef it is no longer bound. Retained for backward
	// compatibility; removal is a follow-up.
	ClusterRoles []string          `json:"clusterRoles,omitempty"`
	Config       map[string]string `json:"config,omitempty"`
}

// DefinitionRef pins an immutable, content-addressed PluginDefinition.
// A published PluginDefinition is content-addressed: (PluginName,
// PluginVersion) resolves to exactly one DefinitionHash forever, so the pin is
// itself the consent record (FUN-17 "Where the scope comes from").
//
// +k8s:deepcopy-gen=true
type DefinitionRef struct {
	PluginName     string `json:"pluginName"`
	PluginVersion  string `json:"pluginVersion"`
	// DefinitionHash is the admin's install-time consent record: plugin-controller
	// enforces that the plugin's own GetDefinition RPC hashes to this value
	// before materialising the plugin-scope ClusterRole. The literal
	// "sha256:mock" is a reserved sentinel that bypasses the check — used in
	// local dev where computing a real hash is friction with no marketplace
	// integration.
	DefinitionHash string `json:"definitionHash"`
}

type PluginInstallationStatus struct {
	Phase              PluginPhase `json:"phase,omitempty"`
	Message            string      `json:"message,omitempty"`
	Ready              bool        `json:"ready,omitempty"`
	ObservedGeneration int64       `json:"observedGeneration,omitempty"`
}

// PluginInstallationList is a list of PluginInstallation resources.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PluginInstallationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PluginInstallation `json:"items"`
}
