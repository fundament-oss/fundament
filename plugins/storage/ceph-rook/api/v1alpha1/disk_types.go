//go:generate controller-gen object paths=.
//go:generate controller-gen crd paths=. output:crd:dir=../../crds

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// DiskType classifies a block device.
// +kubebuilder:validation:Enum=hdd;ssd;nvme
type DiskType string

const (
	DiskTypeHDD  DiskType = "hdd"
	DiskTypeSSD  DiskType = "ssd"
	DiskTypeNVMe DiskType = "nvme"
)

// DiskSpec is intentionally empty: Disk objects are published by the plugin
// from Rook device discovery and are not operator-editable.
type DiskSpec struct{}

// DiskStatus is the discovered state of a node block device.
type DiskStatus struct {
	Node string `json:"node,omitempty"`
	// Path is the kernel device name, e.g. /dev/sdb. It is what an operator
	// recognises, but the kernel is free to reassign it across reboots, so it
	// is not what this plugin identifies the device by.
	Path string `json:"path,omitempty"`
	// StablePath is the /dev/disk/by-id link for this device, empty when the
	// node reports none (loop devices, some virtual disks). When set it is what
	// goes into CephCluster.spec.storage.nodes, because it survives a reboot
	// that renames Path.
	StablePath string   `json:"stablePath,omitempty"`
	SizeBytes  int64    `json:"sizeBytes,omitempty"`
	Type       DiskType `json:"type,omitempty"`
	Rotational bool     `json:"rotational,omitempty"`
	Model      string   `json:"model,omitempty"`
	Serial     string   `json:"serial,omitempty"`
	// WWN is the device's World Wide Name, a fallback stable identity when the
	// node exposes no by-id link.
	WWN       string `json:"wwn,omitempty"`
	Available bool   `json:"available,omitempty"`
	ClaimedBy string `json:"claimedBy,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.status.node`
// +kubebuilder:printcolumn:name="Path",type=string,JSONPath=`.status.path`
// +kubebuilder:printcolumn:name="Size",type=integer,JSONPath=`.status.sizeBytes`
// +kubebuilder:printcolumn:name="Available",type=boolean,JSONPath=`.status.available`
type Disk struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DiskSpec   `json:"spec,omitempty"`
	Status            DiskStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type DiskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Disk `json:"items"`
}

func init() { SchemeBuilder.Register(&Disk{}, &DiskList{}) }
