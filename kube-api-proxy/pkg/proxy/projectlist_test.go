package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/gardener"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kube"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kubereq"
	tokenpkg "github.com/fundament-oss/fundament/kube-api-proxy/pkg/token"
)

func TestParseProjectList(t *testing.T) {
	t.Parallel()
	pid := uuid.New()
	p := "fundamentProjectId=" + pid.String()

	tests := []struct {
		name      string
		method    string
		target    string
		wantOK    bool
		wantErr   bool
		wantQuery string
	}{
		{"cluster-wide list", http.MethodGet, "/apis/cert-manager.io/v1/certificates?" + p, true, false, ""},
		{"other params are kept", http.MethodGet, "/apis/cert-manager.io/v1/certificates?labelSelector=a%3Db&" + p, true, false, "labelSelector=a%3Db"},
		{"watch=false is a list", http.MethodGet, "/apis/cert-manager.io/v1/certificates?watch=false&" + p, true, false, "watch=false"},
		{"namespaced path ignores the param", http.MethodGet, "/apis/cert-manager.io/v1/namespaces/x/certificates?" + p, false, false, ""},
		{"named get ignores the param", http.MethodGet, "/apis/cert-manager.io/v1/certificates/a?" + p, false, false, ""},
		{"core cluster-wide list is rejected", http.MethodGet, "/api/v1/pods?" + p, false, true, ""},
		{"core namespaced path ignores the param", http.MethodGet, "/api/v1/namespaces/x/pods?" + p, false, false, ""},
		{"core named get ignores the param", http.MethodGet, "/api/v1/namespaces/x/pods/a?" + p, false, false, ""},
		{"discovery is rejected", http.MethodGet, "/apis/cert-manager.io/v1?" + p, false, true, ""},
		{"core discovery is rejected", http.MethodGet, "/api/v1?" + p, false, true, ""},
		{"POST is rejected", http.MethodPost, "/apis/cert-manager.io/v1/certificates?" + p, false, true, ""},
		{"collection DELETE is rejected", http.MethodDelete, "/apis/cert-manager.io/v1/certificates?" + p, false, true, ""},
		{"PATCH is rejected", http.MethodPatch, "/apis/cert-manager.io/v1/namespaces/x/certificates/a?" + p, false, true, ""},
		{"no param", http.MethodGet, "/apis/cert-manager.io/v1/certificates?limit=10", false, false, "limit=10"},
		{"bad uuid", http.MethodGet, "/apis/cert-manager.io/v1/certificates?fundamentProjectId=nope", false, true, ""},
		{"watch", http.MethodGet, "/apis/cert-manager.io/v1/certificates?watch=true&" + p, false, true, "watch=true"},
		{"limit", http.MethodGet, "/apis/cert-manager.io/v1/certificates?limit=10&" + p, false, true, "limit=10"},
		{"continue", http.MethodGet, "/apis/cert-manager.io/v1/certificates?continue=x&" + p, false, true, "continue=x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequestWithContext(context.Background(), tt.method, tt.target, http.NoBody)
			got, ok, err := parseProjectList(r)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, pid, got)
			}
			assert.Equal(t, tt.wantQuery, r.URL.RawQuery)
		})
	}
}

func TestParseProjectList_RejectsImpersonation(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet,
		"/apis/cert-manager.io/v1/certificates?fundamentProjectId="+uuid.NewString(), http.NoBody)
	r.Header.Set("Impersonate-User", "someone-else")
	_, ok, err := parseProjectList(r)
	require.Error(t, err)
	assert.False(t, ok)
}

// listForward answers every namespaced list with one item in that namespace
// and records the forwarded requests. It is called concurrently.
type listForward struct {
	mu   sync.Mutex
	reqs []*http.Request
	// fail maps a namespace to the status it answers instead.
	fail map[string]int
}

func (f *listForward) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.reqs = append(f.reqs, r)
	f.mu.Unlock()
	ns := strings.Split(r.URL.Path, "/")[5] // /apis/{g}/{v}/namespaces/{ns}/{plural}
	if status, ok := f.fail[ns]; ok {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(http.StatusText(status)))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"apiVersion":"cert-manager.io/v1","kind":"CertificateList","metadata":{"resourceVersion":"9"},"items":[{"metadata":{"name":"a","namespace":%q}}]}`, ns) //nolint:gosec // test fake, ns is from the test's own request
}

// sorted returns the forwarded requests by path: they arrive in any order.
func (f *listForward) sorted() []*http.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.SortedFunc(slices.Values(f.reqs), func(a, b *http.Request) int {
		return strings.Compare(a.URL.Path, b.URL.Path)
	})
}

// itemNamespaces decodes a merged list into its items' namespaces, in order.
func itemNamespaces(t *testing.T, body []byte) []string {
	t.Helper()
	var got struct {
		Items []struct {
			Metadata struct {
				Namespace string `json:"namespace"`
			} `json:"metadata"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(body, &got))
	out := make([]string, 0, len(got.Items))
	for _, item := range got.Items {
		out = append(out, item.Metadata.Namespace)
	}
	return out
}

func newProjectListRequest() *http.Request {
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/apis/cert-manager.io/v1/certificates?labelSelector=a%3Db", http.NoBody)
	r.Header.Set("Accept-Encoding", "gzip")
	return r
}

func TestServeProjectList_Merges(t *testing.T) {
	t.Parallel()
	f := &listForward{}
	rec := httptest.NewRecorder()
	serveProjectList(rec, newProjectListRequest(), []string{"ns1", "ns2"}, f.serve)

	reqs := f.sorted()
	require.Len(t, reqs, 2)
	assert.Equal(t, "/apis/cert-manager.io/v1/namespaces/ns1/certificates", reqs[0].URL.Path)
	assert.Equal(t, "/apis/cert-manager.io/v1/namespaces/ns2/certificates", reqs[1].URL.Path)
	for _, r := range reqs {
		assert.NotContains(t, r.URL.RawQuery, projectQueryParam)
		assert.Equal(t, "labelSelector=a%3Db", r.URL.RawQuery)
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		assert.Empty(t, r.Header.Get("Accept-Encoding"), "sub-responses must be readable")
	}

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	var got struct {
		APIVersion string            `json:"apiVersion"`
		Kind       string            `json:"kind"`
		Metadata   map[string]any    `json:"metadata"`
		Items      []json.RawMessage `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "cert-manager.io/v1", got.APIVersion)
	assert.Equal(t, "CertificateList", got.Kind)
	assert.Equal(t, map[string]any{}, got.Metadata)
	assert.Len(t, got.Items, 2)
}

func TestProjectList_NoNamespaces(t *testing.T) {
	t.Parallel()
	f := &listForward{}
	rec := httptest.NewRecorder()
	serveProjectList(rec, newProjectListRequest(), nil, f.serve)

	assert.Empty(t, f.reqs)
	assert.Equal(t, http.StatusOK, rec.Code)
	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.JSONEq(t, `[]`, string(got["items"]))
	assert.JSONEq(t, `{}`, string(got["metadata"]))
}

func TestServeProjectList_KeepsNamespaceOrder(t *testing.T) {
	t.Parallel()
	namespaces := []string{"ns5", "ns1", "ns4", "ns2", "ns3", "ns6"}
	f := &listForward{}
	rec := httptest.NewRecorder()
	serveProjectList(rec, newProjectListRequest(), namespaces, f.serve)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, namespaces, itemNamespaces(t, rec.Body.Bytes()))
}

func TestServeProjectList_SubrequestError(t *testing.T) {
	t.Parallel()
	f := &listForward{fail: map[string]int{"ns2": http.StatusInternalServerError}}
	rec := httptest.NewRecorder()
	serveProjectList(rec, newProjectListRequest(), []string{"ns1", "ns2", "ns3"}, f.serve)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "Internal Server Error", rec.Body.String())
}

func TestServeProjectList_LeavesOutForbiddenNamespaces(t *testing.T) {
	t.Parallel()
	f := &listForward{fail: map[string]int{"ns2": http.StatusForbidden}}
	rec := httptest.NewRecorder()
	serveProjectList(rec, newProjectListRequest(), []string{"ns1", "ns2", "ns3"}, f.serve)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"ns1", "ns3"}, itemNamespaces(t, rec.Body.Bytes()))
	assert.Equal(t, []string{`299 - "namespace ns2 left out: forbidden"`}, rec.Header().Values("Warning"))
}

func TestServeProjectList_AllForbidden(t *testing.T) {
	t.Parallel()
	f := &listForward{fail: map[string]int{"ns1": http.StatusForbidden, "ns2": http.StatusForbidden}}
	rec := httptest.NewRecorder()
	serveProjectList(rec, newProjectListRequest(), []string{"ns1", "ns2"}, f.serve)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Empty(t, rec.Header().Values("Warning"))
}

func TestServeProjectList_TooLarge(t *testing.T) {
	t.Parallel()
	chunk := bytes.Repeat([]byte(" "), 1<<20)
	var calls atomic.Int32
	rec := httptest.NewRecorder()
	serveProjectList(rec, newProjectListRequest(), []string{"ns1", "ns2", "ns3", "ns4", "ns5", "ns6"}, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		for range 16 {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "larger than 32 MiB")
	assert.Less(t, calls.Load(), int32(6), "the namespaces not yet asked are skipped")
}

func TestServeProjectList_AbortedSubrequest(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	serveProjectList(rec, newProjectListRequest(), []string{"ns1", "ns2"}, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/ns2/") {
			panic(http.ErrAbortHandler)
		}
		_, _ = w.Write([]byte(`{"items":[]}`))
	})
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestServeProjectList_PanicReachesHandler(t *testing.T) {
	t.Parallel()
	assert.PanicsWithValue(t, "boom", func() {
		serveProjectList(httptest.NewRecorder(), newProjectListRequest(), []string{"ns1"}, func(http.ResponseWriter, *http.Request) {
			panic("boom")
		})
	})
}

func TestServeProjectList_InvalidJSON(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	serveProjectList(rec, newProjectListRequest(), []string{"ns1"}, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	})
	assert.Equal(t, http.StatusBadGateway, rec.Code)
}

func TestServeProjectList_RejectsNonJSONAccept(t *testing.T) {
	t.Parallel()
	for _, accept := range []string{
		"application/json;as=Table;v=v1;g=meta.k8s.io,application/json",
		"application/vnd.kubernetes.protobuf",
	} {
		f := &listForward{}
		r := newProjectListRequest()
		r.Header.Set("Accept", accept)
		rec := httptest.NewRecorder()
		serveProjectList(rec, r, []string{"ns1"}, f.serve)
		assert.Equal(t, http.StatusNotAcceptable, rec.Code, accept)
		assert.Empty(t, f.reqs, accept)
	}
}

// recordingUserSAR allows everything and records what it reviewed. It is
// called concurrently.
type recordingUserSAR struct {
	mu    sync.Mutex
	attrs []kubereq.Attributes
}

func (s *recordingUserSAR) Check(_ context.Context, _ string, attrs *kubereq.Attributes, _ string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attrs = append(s.attrs, *attrs)
	return true, nil
}

func TestPluginGateway_ProjectList(t *testing.T) {
	t.Parallel()
	secret := []byte("s")
	clusterID, userID, pid := uuid.New(), uuid.New(), uuid.New()
	sar := &recordingUserSAR{}
	f := &listForward{}
	g := &pluginGateway{
		logger:      discardLogger(),
		jwtSecret:   secret,
		userSAR:     sar,
		pluginSA:    stubPluginSA{},
		canView:     func(context.Context, uuid.UUID, uuid.UUID) (bool, error) { return true, nil },
		kubeHandler: http.HandlerFunc(f.serve),
		projectNamespaces: func(_ context.Context, u, c, p uuid.UUID) ([]string, error) {
			assert.Equal(t, userID, u)
			assert.Equal(t, clusterID, c)
			assert.Equal(t, pid, p)
			return []string{"ns1", "ns2"}, nil
		},
	}

	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/apis/cert-manager.io/v1/certificates?fundamentProjectId="+pid.String(), http.NoBody)
	r.Header.Set("Authorization", "Bearer "+mintPluginToken(t, secret, userID.String(), clusterID.String()))
	w := httptest.NewRecorder()
	g.serve(w, r, clusterID.String())

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	require.Len(t, sar.attrs, 2, "one user SAR per namespace")
	attrs := slices.SortedFunc(slices.Values(sar.attrs), func(a, b kubereq.Attributes) int {
		return strings.Compare(a.Namespace, b.Namespace)
	})
	reqs := f.sorted()
	for i, ns := range []string{"ns1", "ns2"} {
		assert.Equal(t, kubereq.Attributes{APIGroup: "cert-manager.io", Namespace: ns, Resource: "certificates", Verb: "list"}, attrs[i])
		assert.Equal(t, "/apis/cert-manager.io/v1/namespaces/"+ns+"/certificates", reqs[i].URL.Path)
		assert.Equal(t, "plugin-sa-token", SATokenFrom(reqs[i].Context()))
	}
	assert.Contains(t, w.Body.String(), `"namespace":"ns2"`)
}

func TestPluginGateway_ProjectListMockMode(t *testing.T) {
	t.Parallel()
	secret := []byte("s")
	clusterID := uuid.NewString()
	g := newPluginGateway(t, secret, true, true)

	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/apis/cert-manager.io/v1/certificates?fundamentProjectId="+uuid.NewString(), http.NoBody)
	r.Header.Set("Authorization", "Bearer "+mintPluginToken(t, secret, uuid.NewString(), clusterID))
	w := httptest.NewRecorder()
	g.serve(w, r, clusterID)

	require.Equal(t, http.StatusOK, w.Code)
	fwd := g.lastForwarded()
	require.NotNil(t, fwd)
	assert.Equal(t, "/apis/cert-manager.io/v1/certificates", fwd.URL.Path)
	assert.Empty(t, fwd.URL.RawQuery)
}

func TestPluginGateway_ProjectListBadRequest(t *testing.T) {
	t.Parallel()
	secret := []byte("s")
	clusterID := uuid.NewString()
	g := newPluginGateway(t, secret, true, true)

	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/apis/cert-manager.io/v1/certificates?watch=1&fundamentProjectId="+uuid.NewString(), http.NoBody)
	r.Header.Set("Authorization", "Bearer "+mintPluginToken(t, secret, uuid.NewString(), clusterID))
	w := httptest.NewRecorder()
	g.serve(w, r, clusterID)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Nil(t, g.lastForwarded())
}

func mintUserToken(t *testing.T, secret []byte, userID uuid.UUID) string {
	t.Helper()
	c := &auth.Claims{RegisteredClaims: jwt.RegisteredClaims{
		Subject:   userID.String(),
		Issuer:    auth.ConsoleIssuer,
		Audience:  jwt.ClaimStrings{auth.TokenTypeUser},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(secret)
	require.NoError(t, err)
	return s
}

// fakeSARequester issues a fixed SA token.
type fakeSARequester struct{ token string }

func (f fakeSARequester) RequestNamedSAToken(context.Context, string, string) (*gardener.SAToken, error) {
	return &gardener.SAToken{Token: f.token, ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func newUserPathServer(secret []byte, kubeHandler http.Handler, namespaces *namespaceLister) *Server {
	return &Server{
		logger:        discardLogger(),
		authValidator: auth.NewValidatorForAudience(secret, auth.ConsoleAuthCookieName, auth.ConsoleIssuer, auth.TokenTypeUser, discardLogger()),
		kubeHandler:   kubeHandler,
		namespaces:    namespaces,
	}
}

func TestUserPath_ProjectList(t *testing.T) {
	t.Parallel()
	secret := []byte("s")
	clusterID, userID, pid := uuid.New(), uuid.New(), uuid.New()
	nsUp := &namespaceListUpstream{names: []string{"ns2", "ns1"}}
	f := &listForward{}
	s := newUserPathServer(secret, http.HandlerFunc(f.serve),
		newTestLister(&fakeVisibilityAuthz{projects: []string{pid.String()}}, nil, nsUp))
	s.tokenCache = tokenpkg.NewCache(fakeSARequester{token: "user-sa-token"}, discardLogger())

	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/apis/cert-manager.io/v1/certificates?fundamentProjectId="+pid.String(), http.NoBody)
	r.Header.Set("Authorization", "Bearer "+mintUserToken(t, secret, userID))
	w := httptest.NewRecorder()
	s.handleUserClusterProxy(w, r, clusterID)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	assert.Contains(t, nsUp.req.URL.Query().Get("labelSelector"), "fundament.io/project-id="+pid.String())
	reqs := f.sorted()
	require.Len(t, reqs, 2)
	for i, ns := range []string{"ns1", "ns2"} {
		assert.Equal(t, "/apis/cert-manager.io/v1/namespaces/"+ns+"/certificates", reqs[i].URL.Path)
		assert.Equal(t, clusterID.String(), reqs[i].Context().Value(kube.ClusterIDContextKey{}))
		_, impersonated := kube.ImpersonationFrom(reqs[i].Context())
		assert.False(t, impersonated, "items are read as the user, not the namespace lister")
		assert.Equal(t, "user-sa-token", SATokenFrom(reqs[i].Context()))
	}
}

func TestUserPath_ProjectListMockMode(t *testing.T) {
	t.Parallel()
	secret := []byte("s")
	var fwd *http.Request
	s := newUserPathServer(secret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fwd = r
		w.WriteHeader(http.StatusOK)
	}), nil)

	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/apis/cert-manager.io/v1/certificates?fundamentProjectId="+uuid.NewString(), http.NoBody)
	r.Header.Set("Authorization", "Bearer "+mintUserToken(t, secret, uuid.New()))
	w := httptest.NewRecorder()
	s.handleUserClusterProxy(w, r, uuid.New())

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, fwd)
	assert.Equal(t, "/apis/cert-manager.io/v1/certificates", fwd.URL.Path)
	assert.Empty(t, fwd.URL.RawQuery)

	r = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/apis/cert-manager.io/v1/certificates?limit=5&fundamentProjectId="+uuid.NewString(), http.NoBody)
	r.Header.Set("Authorization", "Bearer "+mintUserToken(t, secret, uuid.New()))
	w = httptest.NewRecorder()
	s.handleUserClusterProxy(w, r, uuid.New())
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
