package useraccess

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// TestSandboxClient_PostsSARAndReturnsDecision confirms the sandbox client
// forwards the user/groups/attrs to a real SubjectAccessReview (no allow-all
// shortcut) and returns the apiserver's Allowed decision, ignoring clusterID.
func TestSandboxClient_PostsSARAndReturnsDecision(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	var captured *authorizationv1.SubjectAccessReview
	cs.PrependReactor("create", "subjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		sar := action.(k8stesting.CreateAction).GetObject().(*authorizationv1.SubjectAccessReview)
		captured = sar
		sar.Status.Allowed = true
		return true, sar, nil
	})

	c := &sandboxClient{cs: cs}
	attrs := authorizationv1.ResourceAttributes{
		Verb:     "list",
		Group:    "cert-manager.io",
		Resource: "certificates",
	}
	allowed, err := c.SubjectAccessReview(
		context.Background(),
		"ignored-cluster",
		"system:serviceaccount:fundament-system:fundament-abc",
		[]string{"system:authenticated"},
		attrs,
	)
	require.NoError(t, err)
	assert.True(t, allowed)

	require.NotNil(t, captured, "a SubjectAccessReview must actually be posted")
	assert.Equal(t, "system:serviceaccount:fundament-system:fundament-abc", captured.Spec.User)
	assert.Equal(t, []string{"system:authenticated"}, captured.Spec.Groups)
	require.NotNil(t, captured.Spec.ResourceAttributes)
	assert.Equal(t, attrs, *captured.Spec.ResourceAttributes)
}

// TestSandboxClient_DeniedIsNotError confirms a denied SAR surfaces as false,
// not an error.
func TestSandboxClient_DeniedIsNotError(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	cs.PrependReactor("create", "subjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		sar := action.(k8stesting.CreateAction).GetObject().(*authorizationv1.SubjectAccessReview)
		sar.Status.Allowed = false
		return true, sar, nil
	})

	c := &sandboxClient{cs: cs}
	allowed, err := c.SubjectAccessReview(
		context.Background(),
		"ignored",
		"system:serviceaccount:fundament-system:fundament-abc",
		nil,
		authorizationv1.ResourceAttributes{Verb: "delete", Resource: "secrets"},
	)
	require.NoError(t, err)
	assert.False(t, allowed)
}
