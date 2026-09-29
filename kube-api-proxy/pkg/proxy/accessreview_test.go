package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kube"
)

// reviewUpstream answers every access review with the given attributes and
// decision, recording what it was sent.
type reviewUpstream struct {
	attrs   authorizationv1.ResourceAttributes
	allowed bool
	status  int
	req     *http.Request
}

func (u *reviewUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u.req = r
	sar := authorizationv1.SelfSubjectAccessReview{
		TypeMeta: metav1.TypeMeta{APIVersion: "authorization.k8s.io/v1", Kind: "SelfSubjectAccessReview"},
		Spec:     authorizationv1.SelfSubjectAccessReviewSpec{ResourceAttributes: &u.attrs},
		Status:   authorizationv1.SubjectAccessReviewStatus{Allowed: u.allowed},
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Audit-Id", "a1")
	w.WriteHeader(u.status)
	_ = json.NewEncoder(w).Encode(&sar)
}

func runReview(t *testing.T, up *reviewUpstream) (*httptest.ResponseRecorder, authorizationv1.SelfSubjectAccessReview) {
	t.Helper()
	l := newTestLister(&fakeVisibilityAuthz{}, nil, up)
	cluster := uuid.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, accessReviewPath, strings.NewReader(`{}`))
	req.Header.Set("Accept", "application/vnd.kubernetes.protobuf, */*")
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	l.serveAccessReview(rec, req, cluster)

	require.NotNil(t, up.req)
	assert.Equal(t, "application/json", up.req.Header.Get("Accept"))
	assert.Empty(t, up.req.Header.Get("Accept-Encoding"))
	assert.Equal(t, cluster.String(), up.req.Context().Value(kube.ClusterIDContextKey{}))

	var got authorizationv1.SelfSubjectAccessReview
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return rec, got
}

func TestServeAccessReview_AllowsClusterNamespaceList(t *testing.T) {
	t.Parallel()
	for _, attrs := range []authorizationv1.ResourceAttributes{
		{Verb: "list", Resource: "namespaces"},                                        // k9s
		{Verb: "watch", Resource: "namespaces"},                                       // informers
		{Verb: "list", Resource: "namespaces", Namespace: "default"},                  // kubectl auth can-i
		{Verb: "list", Resource: "namespaces", Namespace: "default", Name: "default"}, // k9s namespace view
	} {
		rec, got := runReview(t, &reviewUpstream{attrs: attrs, status: http.StatusCreated})
		assert.Equal(t, http.StatusCreated, rec.Code)
		assert.True(t, got.Status.Allowed, "%+v", attrs)
		assert.Equal(t, namespaceListAllowedReason, got.Status.Reason)
		assert.Equal(t, "SelfSubjectAccessReview", got.Kind)
		assert.Equal(t, "a1", rec.Header().Get("Audit-Id"))
	}
}

func TestServeAccessReview_LeavesOtherReviewsAlone(t *testing.T) {
	t.Parallel()
	for name, attrs := range map[string]authorizationv1.ResourceAttributes{
		"pods":              {Verb: "list", Resource: "pods"},
		"named namespace":   {Verb: "get", Resource: "namespaces", Name: "kube-system"},
		"delete namespaces": {Verb: "delete", Resource: "namespaces"},
		"other group":       {Verb: "list", Group: "example.com", Resource: "namespaces"},
		"subresource":       {Verb: "list", Resource: "namespaces", Subresource: "status"},
	} {
		_, got := runReview(t, &reviewUpstream{attrs: attrs, status: http.StatusCreated})
		assert.False(t, got.Status.Allowed, name)
	}

	// Errors pass through untouched.
	rec, got := runReview(t, &reviewUpstream{
		attrs:  authorizationv1.ResourceAttributes{Verb: "list", Resource: "namespaces"},
		status: http.StatusForbidden,
	})
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.False(t, got.Status.Allowed)
}

func TestIsSelfSubjectAccessReview(t *testing.T) {
	t.Parallel()
	post := httptest.NewRequestWithContext(context.Background(), http.MethodPost, accessReviewPath, http.NoBody)
	assert.True(t, isSelfSubjectAccessReview(post))
	rules := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/apis/authorization.k8s.io/v1/selfsubjectrulesreviews", http.NoBody)
	assert.False(t, isSelfSubjectAccessReview(rules))
}
