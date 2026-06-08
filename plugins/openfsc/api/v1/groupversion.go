// Package v1 contains the openfsc.fundament.io/v1 API types. The OpenFSC plugin
// owns cluster-scoped CRDs: Peer (a member of an FSC group; the plugin
// auto-seeds the local cluster's directory as the "self" peer), and the gateway
// resources Inway and Outway, whose registration with the OpenFSC Controller the
// operator observes.
//
// +kubebuilder:object:generate=true
// +groupName=openfsc.fundament.io
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupVersion is the API group/version for the OpenFSC plugin CRDs.
var GroupVersion = schema.GroupVersion{Group: "openfsc.fundament.io", Version: "v1"}

var (
	// SchemeBuilder registers the openfsc.fundament.io/v1 types with a scheme.
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	// AddToScheme adds the openfsc.fundament.io/v1 types to a scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion,
		&Peer{}, &PeerList{},
		&Inway{}, &InwayList{},
		&Outway{}, &OutwayList{},
	)
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}
