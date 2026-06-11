package installation

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
)

// mockInstallationUID is the UID of the cert-manager PluginInstallation in the
// mock cluster. authn-api's MintPluginToken uses it as the installation_id.
const mockInstallationUID = "00000000-0000-0000-0000-000000000001"

// NewMockClusterClient returns a ClusterClientFn serving one in-memory cluster
// with a single cert-manager PluginInstallation. Used for local dev; real-mode
// (Gardener admin-kubeconfig) cluster access lands in Plan C.
func NewMockClusterClient() ClusterClientFn {
	scheme := runtime.NewScheme()
	_ = pluginsv1.AddToScheme(scheme)
	cr := &pluginsv1.PluginInstallation{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cert-manager",
			UID:  types.UID(mockInstallationUID),
		},
		Spec: pluginsv1.PluginInstallationSpec{
			Image: "ghcr.io/example/cert-manager:v1.17.2",
			DefinitionRef: pluginsv1.DefinitionRef{
				PluginName:     "cert-manager",
				PluginVersion:  "v1.17.2",
				DefinitionHash: "sha256:mock",
			},
		},
		Status: pluginsv1.PluginInstallationStatus{Phase: pluginsv1.PluginPhaseRunning},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).Build()
	return func(_ context.Context, _ string) (client.Client, error) { return c, nil }
}

// NewMockOrgIDResolver returns a fixed organization ID for every cluster.
func NewMockOrgIDResolver() OrgIDForClusterFn {
	return func(_ context.Context, _ string) (string, error) {
		return "00000000-0000-0000-0000-000000000000", nil
	}
}
