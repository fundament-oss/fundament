package useraccess

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authorizationv1 "k8s.io/api/authorization/v1"

	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kubereq"
)

// fakeClient records the SAR invocation and returns a canned response.
type fakeClient struct {
	allowed bool
	err     error

	lastClusterID string
	lastUser      string
	lastAttrs     authorizationv1.ResourceAttributes
	calls         int
}

func (f *fakeClient) SubjectAccessReview(_ context.Context, clusterID, user string, attrs authorizationv1.ResourceAttributes) (bool, error) {
	f.calls++
	f.lastClusterID = clusterID
	f.lastUser = user
	f.lastAttrs = attrs
	if f.err != nil {
		return false, f.err
	}
	return f.allowed, nil
}

func TestStub_AlwaysAllows(t *testing.T) {
	allowed, err := Stub{}.Check(context.Background(), "any-cluster", &kubereq.Attributes{Verb: "delete"}, "any-user")
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestReal_ForwardsAttrsAndReturnsAllowed(t *testing.T) {
	fake := &fakeClient{allowed: true}
	r := New(fake, nil)

	attrs := &kubereq.Attributes{
		APIGroup:    "cert-manager.io",
		Resource:    "certificates",
		Subresource: "status",
		Name:        "wildcard",
		Namespace:   "team-foo",
		Verb:        "get",
	}
	allowed, err := r.Check(context.Background(), "cluster-abc", attrs, "019b4000-1000-7000-8000-000000000001")
	require.NoError(t, err)
	assert.True(t, allowed)

	assert.Equal(t, 1, fake.calls)
	assert.Equal(t, "cluster-abc", fake.lastClusterID)
	assert.Equal(t, "system:serviceaccount:fundament-system:fundament-019b4000-1000-7000-8000-000000000001", fake.lastUser,
		"SAR user is the per-user SA cluster-worker provisions on the tenant cluster")
	assert.Equal(t, authorizationv1.ResourceAttributes{
		Namespace:   "team-foo",
		Verb:        "get",
		Group:       "cert-manager.io",
		Resource:    "certificates",
		Subresource: "status",
		Name:        "wildcard",
	}, fake.lastAttrs)
}

func TestReal_DeniedResponseIsNotAnError(t *testing.T) {
	fake := &fakeClient{allowed: false}
	r := New(fake, nil)
	attrs := &kubereq.Attributes{Verb: "delete", Resource: "secrets", Namespace: "kube-system"}
	allowed, err := r.Check(context.Background(), "cluster-abc", attrs, "user-1")
	require.NoError(t, err)
	assert.False(t, allowed, "denied SAR must surface as false, not error")
}

func TestReal_ClientErrorPropagates(t *testing.T) {
	fake := &fakeClient{err: errors.New("gardener boom")}
	r := New(fake, nil)
	attrs := &kubereq.Attributes{Verb: "get"}
	_, err := r.Check(context.Background(), "cluster-abc", attrs, "user-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gardener boom")
}

func TestReal_NilAttrsRejected(t *testing.T) {
	fake := &fakeClient{}
	r := New(fake, nil)
	_, err := r.Check(context.Background(), "cluster-abc", nil, "user-1")
	require.Error(t, err)
	assert.Equal(t, 0, fake.calls, "no SAR sent when attrs is nil")
}
