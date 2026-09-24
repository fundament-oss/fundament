package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/gardener"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kube"
)

type fakeVisibilityAuthz struct {
	admin     bool
	projects  []string
	err       error
	evaluates atomic.Int32
	lists     atomic.Int32
}

func (f *fakeVisibilityAuthz) Evaluate(_ context.Context, req authz.EvaluationRequest) (authz.Decision, error) { //nolint:gocritic // signature fixed by visibilityAuthz
	f.evaluates.Add(1)
	if req.Action.Name != authz.ActionAdmin || req.Resource.Type != authz.ObjectTypeCluster {
		return authz.Decision{}, fmt.Errorf("unexpected check %s on %s", req.Action.Name, req.Resource.Type)
	}
	return authz.Decision{Decision: f.admin}, f.err
}

func (f *fakeVisibilityAuthz) ListObjects(_ context.Context, _ authz.Object, action authz.Action, objectType authz.ObjectType) ([]string, error) {
	f.lists.Add(1)
	if action.Name != authz.ActionCanView || objectType != authz.ObjectTypeProject {
		return nil, fmt.Errorf("unexpected list %s %s", action.Name, objectType)
	}
	return append([]string(nil), f.projects...), f.err
}

func TestVisibilityResolver(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	user, cluster := uuid.New(), uuid.New()

	admin := &fakeVisibilityAuthz{admin: true}
	vis, err := newVisibilityResolver(admin).resolve(ctx, user, cluster)
	require.NoError(t, err)
	assert.Equal(t, visibility{admin: true}, vis)
	assert.Zero(t, admin.lists.Load(), "admins never list projects")

	member := &fakeVisibilityAuthz{projects: []string{"p2", "p1"}}
	r := newVisibilityResolver(member)
	vis, err = r.resolve(ctx, user, cluster)
	require.NoError(t, err)
	assert.Equal(t, visibility{projectIDs: []string{"p1", "p2"}}, vis, "sorted for a stable selector")
	_, err = r.resolve(ctx, user, cluster)
	require.NoError(t, err)
	assert.Equal(t, int32(1), member.lists.Load(), "second lookup served from cache")

	_, err = newVisibilityResolver(&fakeVisibilityAuthz{err: errors.New("openfga down")}).resolve(ctx, user, cluster)
	require.ErrorContains(t, err, "openfga down")
}

func TestRewriteNamespaceQuery(t *testing.T) {
	t.Parallel()
	member := visibility{projectIDs: []string{"p1", "p2"}}

	tests := []struct {
		name  string
		query string
		vis   visibility
		want  url.Values
	}{
		{"admin list is untouched", "limit=500", visibility{admin: true}, url.Values{"limit": {"500"}}},
		{"admin watch keeps its timeout", "watch=1&timeoutSeconds=1800", visibility{admin: true},
			url.Values{"watch": {"1"}, "timeoutSeconds": {"1800"}}},
		{"member list gets the project requirement", "", member,
			url.Values{"labelSelector": {"fundament.io/project-id in (p1,p2)"}}},
		{"client selector is ANDed", "labelSelector=team%3Da", member,
			url.Values{"labelSelector": {"team=a,fundament.io/project-id in (p1,p2)"}}},
		{"no projects matches nothing", "", visibility{},
			url.Values{"labelSelector": {"fundament.io/no-visible-projects"}}},
		{"member watch without timeout is capped", "watch=true", member,
			url.Values{"watch": {"true"}, "timeoutSeconds": {"300"}, "labelSelector": {"fundament.io/project-id in (p1,p2)"}}},
		{"member watch above the cap is capped", "watch=1&timeoutSeconds=3600", member,
			url.Values{"watch": {"1"}, "timeoutSeconds": {"300"}, "labelSelector": {"fundament.io/project-id in (p1,p2)"}}},
		{"member watch below the cap is kept", "watch=1&timeoutSeconds=60", member,
			url.Values{"watch": {"1"}, "timeoutSeconds": {"60"}, "labelSelector": {"fundament.io/project-id in (p1,p2)"}}},
		{"watch=false is a list", "watch=false", member,
			url.Values{"watch": {"false"}, "labelSelector": {"fundament.io/project-id in (p1,p2)"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/namespaces?"+tt.query, http.NoBody)
			rewriteNamespaceQuery(r, tt.vis)
			assert.Equal(t, tt.want, r.URL.Query())
		})
	}
}

func TestIsNamespaceCollectionGet(t *testing.T) {
	t.Parallel()
	assert.True(t, isNamespaceCollectionGet(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/namespaces?watch=1", http.NoBody)))
	assert.False(t, isNamespaceCollectionGet(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/namespaces/team-a", http.NoBody)))
	assert.False(t, isNamespaceCollectionGet(httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/namespaces", http.NoBody)))
	assert.False(t, isNamespaceCollectionGet(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/namespaces/team-a/pods", http.NoBody)))
}

// recordingUpstream captures what namespaceLister forwards and answers with a
// fixed protobuf body.
type recordingUpstream struct {
	calls int
	req   *http.Request
}

var protobufBody = []byte{0x6b, 0x38, 0x73, 0x00, 0x0a, 0x09, 0x0a, 0x02, 0x76, 0x31}

func (u *recordingUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u.calls++
	u.req = r
	w.Header().Set("Content-Type", "application/vnd.kubernetes.protobuf")
	_, _ = w.Write(protobufBody)
}

func newTestLister(az visibilityAuthz, tokenErr error, upstream http.Handler) *namespaceLister {
	return &namespaceLister{
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		visibility: newVisibilityResolver(az),
		proxyToken: func(context.Context, string) (string, error) {
			if tokenErr != nil {
				return "", tokenErr
			}
			return "proxy-sa-token", nil
		},
		upstream: upstream,
	}
}

func TestNamespaceLister_ForwardsOnceAsImpersonatedUser(t *testing.T) {
	t.Parallel()
	user, cluster := uuid.New(), uuid.New()
	up := &recordingUpstream{}
	l := newTestLister(&fakeVisibilityAuthz{projects: []string{"p1"}}, nil, up)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/namespaces?watch=1", http.NoBody)
	req = req.WithContext(WithSAToken(req.Context(), "user-sa-token"))
	rec := httptest.NewRecorder()
	l.serve(rec, req, user, cluster)

	require.Equal(t, 1, up.calls)
	ctx := up.req.Context()
	assert.Equal(t, "proxy-sa-token", SATokenFrom(ctx), "forwarded as the proxy SA")
	imp, ok := kube.ImpersonationFrom(ctx)
	require.True(t, ok)
	assert.Equal(t, "system:serviceaccount:fundament-system:fundament-"+user.String(), imp.User)
	assert.Equal(t, []string{"fundament:namespace-listers"}, imp.Groups)
	assert.Equal(t, cluster.String(), ctx.Value(kube.ClusterIDContextKey{}))
	assert.Equal(t, "fundament.io/project-id in (p1)", up.req.URL.Query().Get("labelSelector"))
	assert.Equal(t, "300", up.req.URL.Query().Get("timeoutSeconds"))
	assert.Equal(t, "watch=1", req.URL.RawQuery, "the caller's request is not mutated")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/vnd.kubernetes.protobuf", rec.Header().Get("Content-Type"))
	assert.Equal(t, protobufBody, rec.Body.Bytes(), "response streamed byte-for-byte")
}

func TestNamespaceLister_Errors(t *testing.T) {
	t.Parallel()
	user, cluster := uuid.New(), uuid.New()

	up := &recordingUpstream{}
	rec := httptest.NewRecorder()
	newTestLister(&fakeVisibilityAuthz{}, fmt.Errorf("wrapped: %w", gardener.ErrSyncPending), up).
		serve(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/namespaces", http.NoBody), user, cluster)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code, "proxy SA not provisioned yet")
	assert.Zero(t, up.calls)

	rec = httptest.NewRecorder()
	newTestLister(&fakeVisibilityAuthz{err: errors.New("openfga down")}, nil, up).
		serve(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/namespaces", http.NoBody), user, cluster)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Zero(t, up.calls)
}
