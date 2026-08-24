//go:generate controller-gen object paths=.

package v1

import (
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
	// DefinitionRef is the immutable pin to the published PluginDefinition the
	// installer consented to. plugin-controller resolves the definition by
	// DefinitionHash and materialises the plugin SA's Role from it (FUN-17).
	// It is the source of truth for the plugin's RBAC scope — no RBAC is
	// copied onto this CR.
	//
	// A plugin's identity is the pair (OrganizationName, PluginName), so
	// metadata.name must equal "<organizationName>--<pluginName>". That makes the
	// apiserver enforce pair-uniqueness, and keeps metadata.name the single
	// handle every consumer addresses the installation by.
	DefinitionRef DefinitionRef     `json:"definitionRef"`
	Config        map[string]string `json:"config,omitempty"`
}

// DefinitionRef pins an immutable, content-addressed PluginDefinition.
// A published PluginDefinition is content-addressed: (PluginName,
// PluginVersion) resolves to exactly one DefinitionHash forever, so the pin is
// itself the consent record (FUN-17 "Where the scope comes from").
//
// +k8s:deepcopy-gen=true
type DefinitionRef struct {
	// OrganizationName is the publishing organization (tenant.organizations.name).
	// Two organizations may publish a plugin with the same PluginName.
	OrganizationName string `json:"organizationName"`
	PluginName       string `json:"pluginName"`
	PluginVersion    string `json:"pluginVersion"`
	// DefinitionHash is the admin's install-time consent record: plugin-controller
	// enforces that the sha256 of the published manifest bytes stored in
	// organization-api matches this value before materialising the plugin-scope
	// ClusterRole. May be omitted only when the controller Deployment sets
	// PLUGIN_CONTROLLER_ALLOW_UNPINNED_HASH=true — for local development.
	DefinitionHash string `json:"definitionHash"`
}

// +k8s:deepcopy-gen=true
type PluginInstallationStatus struct {
	Phase              PluginPhase        `json:"phase,omitempty"`
	Message            string             `json:"message,omitempty"`
	Ready              bool               `json:"ready,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitzero" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
	// Namespace is the resolved cluster-side namespace. Published for operators:
	// a long installation name derives a hashed namespace that cannot be read off
	// metadata.name. Nothing reads it back — derivation stays the authority.
	Namespace string `json:"namespace,omitempty"`
}

// PluginInstallationList is a list of PluginInstallation resources.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PluginInstallationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PluginInstallation `json:"items"`
}
