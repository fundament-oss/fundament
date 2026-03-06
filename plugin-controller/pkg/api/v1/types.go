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

type PluginInstallationSpec struct {
	Image        string            `json:"image"`
	PluginName   string            `json:"pluginName"`
	Version      string            `json:"version"`
	ClusterRoles []string          `json:"clusterRoles,omitempty"`
	Config       map[string]string `json:"config,omitempty"`
}

type PluginInstallationStatus struct {
	Phase              PluginPhase `json:"phase,omitempty"`
	Message            string      `json:"message,omitempty"`
	Ready              bool        `json:"ready,omitempty"`
	ObservedGeneration int64       `json:"observedGeneration,omitempty"`
	PluginVersion      string      `json:"pluginVersion,omitempty"`
}

// PluginInstallationList is a list of PluginInstallation resources.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PluginInstallationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PluginInstallation `json:"items"`
}
