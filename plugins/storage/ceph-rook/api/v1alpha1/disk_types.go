//go:generate controller-gen object paths=.
//go:generate controller-gen crd paths=. output:crd:dir=../../crds

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DiskType classifies a block device.
// +kubebuilder:validation:Enum=hdd;ssd;nvme
type DiskType string

const (
	DiskTypeHDD  DiskType = "hdd"
	DiskTypeSSD  DiskType = "ssd"
	DiskTypeNVMe DiskType = "nvme"
)

// DiskSpec has no fields; disk discovery fills in status.
type DiskSpec struct{}

// DiskStatus is the discovered state of a block device on a cluster node.
type DiskStatus struct {
	// NodeName is the name of the cluster node the device is attached to.
	NodeName string `json:"nodeName,omitempty"`
	// Path is the kernel device name, e.g. /dev/sdb. The kernel may reassign it
	// on reboot; StablePath identifies the device.
	Path string `json:"path,omitempty"`
	// StablePath is the device's /dev/disk/by-id link, empty when the node
	// reports none (loop devices, some virtual disks). It survives reboots, so
	// when set it is the name recorded in the Ceph cluster.
	StablePath string `json:"stablePath,omitempty"`
	// Size is the device's capacity.
	Size resource.Quantity `json:"size,omitempty"`
	// Type classifies the device as hdd, ssd or nvme.
	Type DiskType `json:"type,omitempty"`
	// Rotational is true for spinning disks.
	Rotational bool `json:"rotational,omitempty"`
	// Model is the device model the node reports.
	Model string `json:"model,omitempty"`
	// Serial is the device serial number the node reports.
	Serial string `json:"serial,omitempty"`
	// WWN is the device's World Wide Name, a fallback identity for devices
	// without a by-id link.
	WWN string `json:"wwn,omitempty"`
	// Filesystem is what the last probe found on the device, e.g. "ext4" or
	// "ceph_bluestore". Empty means the probe found none; the probe can be
	// stale.
	Filesystem string `json:"filesystem,omitempty"`
	// Available reports whether the last probe found the device empty and
	// usable. ClaimedBy shows whether a DiskPool has claimed it.
	Available bool `json:"available,omitempty"`
	// ClaimedBy names the DiskPool this disk belongs to, empty when unclaimed.
	ClaimedBy string `json:"claimedBy,omitempty"`
}

// Disk is a block device discovered on a cluster node. Disk discovery creates
// and updates Disks; select them in a DiskPool to use them.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.status.nodeName`
// +kubebuilder:printcolumn:name="Path",type=string,JSONPath=`.status.path`
// +kubebuilder:printcolumn:name="Size",type=string,JSONPath=`.status.size`
// +kubebuilder:printcolumn:name="Available",type=boolean,JSONPath=`.status.available`
// +kubebuilder:printcolumn:name="Filesystem",type=string,JSONPath=`.status.filesystem`
// +kubebuilder:printcolumn:name="Claimed By",type=string,JSONPath=`.status.claimedBy`
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
