// Package v1 contains the openfsc.fundament.io/v1 API types owned by the
// standalone openfsc-operator. All resources are cluster-scoped: Directory (a
// self-contained FSC directory peer the operator deploys), Peer (a member of an
// FSC group; the operator seeds the local directory as the "self" peer), and
// the gateway resources Inway and Outway, which the operator provisions and
// whose registration with the OpenFSC Controller it observes.
//
// +kubebuilder:object:generate=true
// +groupName=openfsc.fundament.io
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupVersion is the API group/version for the OpenFSC operator CRDs.
var GroupVersion = schema.GroupVersion{Group: "openfsc.fundament.io", Version: "v1"}

var (
	// SchemeBuilder registers the openfsc.fundament.io/v1 types with a scheme.
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	// AddToScheme adds the openfsc.fundament.io/v1 types to a scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion,
		&Directory{}, &DirectoryList{},
		&Peer{}, &PeerList{},
		&Inway{}, &InwayList{},
		&Outway{}, &OutwayList{},
	)
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}
