package gardener

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func newPluginInstallationCR(name, uid, hash string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "plugins.fundament.io/v1",
		"kind":       "PluginInstallation",
		"metadata": map[string]any{
			"name": name,
			"uid":  uid,
		},
		"spec": map[string]any{
			"definitionRef": map[string]any{"definitionHash": hash},
		},
	}}
}

func fakeDynamic(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{pluginInstallationGVR: "PluginInstallationList"},
		objs...,
	)
}

// TestResolvePluginSA_GetsByNameAndVerifiesUID verifies the CR is addressed by
// name, its UID is checked against the token's installation_id, and the SA is
// named plugin-{cr.Name}.
func TestResolvePluginSA_GetsByNameAndVerifiesUID(t *testing.T) {
	const uid = "019b4000-2000-7000-8000-000000000009"
	dyn := fakeDynamic(newPluginInstallationCR("cert-manager", uid, "sha256:abc"))

	kube := k8sfake.NewSimpleClientset()
	var gotSAName, gotSANamespace string
	kube.PrependReactor("create", "serviceaccounts", func(action k8stesting.Action) (bool, runtime.Object, error) {
		ca, ok := action.(k8stesting.CreateActionImpl)
		if !ok || ca.GetSubresource() != "token" {
			return false, nil, nil
		}
		gotSAName = ca.Name
		gotSANamespace = ca.GetNamespace()
		return true, &authenticationv1.TokenRequest{
			Status: authenticationv1.TokenRequestStatus{Token: "sa-token-xyz"},
		}, nil
	})

	got, err := resolvePluginSA(context.Background(), dyn, kube, uid, "cert-manager")
	require.NoError(t, err)
	assert.Equal(t, "sa-token-xyz", got.Token)
	assert.Equal(t, "sha256:abc", got.PinnedDefinitionHash)
	assert.Equal(t, "plugin-cert-manager", gotSAName, "SA name derives from CR name")
	assert.Equal(t, "plugin-cert-manager", gotSANamespace)
}

// TestResolvePluginSA_UIDMismatch confirms a token whose installation_id does
// not match the named CR's UID (e.g. deleted-and-recreated install) is rejected.
func TestResolvePluginSA_UIDMismatch(t *testing.T) {
	dyn := fakeDynamic(newPluginInstallationCR("cert-manager", "live-uid", ""))
	kube := k8sfake.NewSimpleClientset()

	_, err := resolvePluginSA(context.Background(), dyn, kube, "stale-uid", "cert-manager")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSyncPending)
	assert.Contains(t, err.Error(), "does not match")
}

// TestResolvePluginSA_MissingCR confirms an unknown installation name is
// reported as ErrSyncPending (the retriable, not-found case).
func TestResolvePluginSA_MissingCR(t *testing.T) {
	dyn := fakeDynamic()
	kube := k8sfake.NewSimpleClientset()

	_, err := resolvePluginSA(context.Background(), dyn, kube, "any-uid", "cert-manager")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSyncPending)
}

// TestResolvePluginSA_MissingName rejects a token that carries no installation
// name (nothing to address the CR by).
func TestResolvePluginSA_MissingName(t *testing.T) {
	dyn := fakeDynamic()
	kube := k8sfake.NewSimpleClientset()

	_, err := resolvePluginSA(context.Background(), dyn, kube, "any-uid", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "installation_name")
}
