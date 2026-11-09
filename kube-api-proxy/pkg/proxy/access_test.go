package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/gardener"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kube"
	tokenpkg "github.com/fundament-oss/fundament/kube-api-proxy/pkg/token"
)

var errNoServiceAccount = fmt.Errorf("service account not found: %w", gardener.ErrSyncPending)

func TestVisibilityResolver_OnCluster(t *testing.T) {
	t.Parallel()
	user, cluster, other := uuid.New(), uuid.New(), uuid.New()
	here, elsewhere := uuid.NewString(), uuid.NewString()
	parents := map[string]string{here: cluster.String(), elsewhere: other.String()}

	tests := []struct {
		name string
		az   *fakeVisibilityAuthz
		want bool
	}{
		{"organization admin", &fakeVisibilityAuthz{admin: true}, true},
		{"member of a project on the cluster", &fakeVisibilityAuthz{projects: []string{elsewhere, here}, parents: parents}, true},
		{"member of a project on another cluster only", &fakeVisibilityAuthz{projects: []string{elsewhere}, parents: parents}, false},
		{"no project", &fakeVisibilityAuthz{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := newVisibilityResolver(tt.az).onCluster(t.Context(), user, cluster)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	_, err := newVisibilityResolver(&fakeVisibilityAuthz{err: errors.New("openfga down")}).onCluster(t.Context(), user, cluster)
	require.ErrorContains(t, err, "openfga down")

	// A refused user retrying in a loop costs OpenFGA one lookup per visibilityTTL.
	az := &fakeVisibilityAuthz{projects: []string{elsewhere}, parents: parents}
	r := newVisibilityResolver(az)
	for range 3 {
		got, err := r.onCluster(t.Context(), user, cluster)
		require.NoError(t, err)
		assert.False(t, got)
	}
	assert.Equal(t, int32(2), az.lists.Load(), "the user's projects and the cluster's, once")
}

type fakeRequester struct {
	token string
	err   error
}

func (f *fakeRequester) RequestNamedSAToken(context.Context, string, string) (*gardener.SAToken, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &gardener.SAToken{Token: f.token, ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func newAccessTestServer(az *fakeVisibilityAuthz, requester tokenpkg.Requester, upstream http.Handler) *Server {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Server{
		logger:      logger,
		tokenCache:  tokenpkg.NewCache(requester, logger),
		kubeHandler: upstream,
		namespaces:  &namespaceLister{logger: logger, visibility: newVisibilityResolver(az)},
	}
}

func decodeStatus(t *testing.T, rec *httptest.ResponseRecorder) metav1.Status {
	t.Helper()
	var status metav1.Status
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &status))
	return status
}

func TestWriteUserTokenError(t *testing.T) {
	t.Parallel()
	user, cluster := uuid.New(), uuid.New()
	project := uuid.NewString()
	member := &fakeVisibilityAuthz{projects: []string{project}, parents: map[string]string{project: cluster.String()}}

	tests := []struct {
		name     string
		az       *fakeVisibilityAuthz
		err      error
		code     int
		contains string
	}{
		{"no role: refused", &fakeVisibilityAuthz{}, errNoServiceAccount, http.StatusForbidden, noAccessMessage},
		{"a role: still being set up", member, errNoServiceAccount, http.StatusServiceUnavailable, "still being set up"},
		{"role unknown: still being set up", &fakeVisibilityAuthz{err: errors.New("openfga down")}, errNoServiceAccount, http.StatusServiceUnavailable, "still being set up"},
		{"other failure: not a matter of roles", &fakeVisibilityAuthz{}, errors.New("connection refused"), http.StatusServiceUnavailable, "details are in the kube-api-proxy logs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := newAccessTestServer(tt.az, &fakeRequester{}, nil)
			rec := httptest.NewRecorder()
			s.writeUserTokenError(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/namespaces/apps/configmaps", http.NoBody), user, cluster, tt.err)
			assert.Equal(t, tt.code, rec.Code)
			assert.Contains(t, decodeStatus(t, rec).Message, tt.contains)
		})
	}
}

func TestRetryUserRequest(t *testing.T) {
	t.Parallel()
	user, cluster := uuid.New(), uuid.New()

	t.Run("ServiceAccount gone with the user's last role: refused", func(t *testing.T) {
		t.Parallel()
		upstream := &recordingUpstream{}
		s := newAccessTestServer(&fakeVisibilityAuthz{}, &fakeRequester{err: errNoServiceAccount}, upstream)
		rec := httptest.NewRecorder()
		s.retryUserRequest(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/namespaces/apps/configmaps", http.NoBody), user, cluster)
		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.Zero(t, upstream.calls)
	})

	t.Run("ServiceAccount created again: sent again with the new token", func(t *testing.T) {
		t.Parallel()
		var token, clusterID string
		upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token = SATokenFrom(r.Context())
			clusterID, _ = r.Context().Value(kube.ClusterIDContextKey{}).(string)
			w.WriteHeader(http.StatusOK)
		})
		s := newAccessTestServer(&fakeVisibilityAuthz{}, &fakeRequester{token: "new-token"}, upstream)
		rec := httptest.NewRecorder()
		s.retryUserRequest(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/namespaces/apps/configmaps", http.NoBody), user, cluster)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "new-token", token)
		assert.Equal(t, cluster.String(), clusterID)
	})

	// HTTP/2 gives every request a Body; ContentLength says whether it has one.
	t.Run("an empty body over HTTP/2 is sent again", func(t *testing.T) {
		t.Parallel()
		upstream := &recordingUpstream{}
		s := newAccessTestServer(&fakeVisibilityAuthz{}, &fakeRequester{token: "new-token"}, upstream)
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/namespaces/apps/configmaps", strings.NewReader(""))
		r.ContentLength = 0
		s.retryUserRequest(httptest.NewRecorder(), r, user, cluster)
		assert.Equal(t, 1, upstream.calls)
	})

	for name, length := range map[string]int64{"a request with a body": 2, "a chunked request": -1} {
		t.Run(name+" is not sent twice", func(t *testing.T) {
			t.Parallel()
			upstream := &recordingUpstream{}
			s := newAccessTestServer(&fakeVisibilityAuthz{}, &fakeRequester{token: "new-token"}, upstream)
			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/namespaces/apps/configmaps", strings.NewReader("{}"))
			r.ContentLength = length
			rec := httptest.NewRecorder()
			s.retryUserRequest(rec, r, user, cluster)
			assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
			assert.Contains(t, decodeStatus(t, rec).Message, "send it again")
			assert.Zero(t, upstream.calls)
		})
	}
}
