package organization

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/organization-api/pkg/gardener"
	"github.com/fundament-oss/fundament/organization-api/pkg/logs"
)

// fakePlutono simulates a shoot Plutono behind the seed ingress basic auth:
// the datasource admin APIs are 403 for the anonymous Viewer, the datasource
// proxy works by numeric id only, id 1 fronts a Prometheus (404 on Vali
// paths), the Vali id is mutable (re-provision drift), and credentials are
// mutable (rotation).
type fakePlutono struct {
	mu     sync.Mutex
	valiID int
	user   string
	pass   string
}

func (f *fakePlutono) handler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		valiID, user, pass := f.valiID, f.user, f.pass
		f.mu.Unlock()

		gotUser, gotPass, ok := r.BasicAuth()
		if !ok || gotUser != user || gotPass != pass {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		const proxyPrefix = "/api/datasources/proxy/"
		if !strings.HasPrefix(r.URL.Path, proxyPrefix) {
			// Everything else on the datasource API is admin-only.
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"Permission denied"}`))
			return
		}

		rest := strings.TrimPrefix(r.URL.Path, proxyPrefix)
		idStr, apiPath, _ := strings.Cut(rest, "/")
		id, err := strconv.Atoi(idStr)
		require.NoError(t, err)

		switch id {
		case valiID:
			f.serveVali(w, "/"+apiPath)
		case 1:
			// Prometheus answers Go-style 404 on Vali paths.
			http.NotFound(w, r)
		default:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"Unable to load datasource meta data"}`))
		}
	})
}

func (f *fakePlutono) serveVali(w http.ResponseWriter, apiPath string) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasPrefix(apiPath, "/vali/api/v1/label/"):
		_, _ = w.Write([]byte(`{"status":"success","data":["kube-system"]}`))
	case apiPath == "/vali/api/v1/query_range":
		_, _ = fmt.Fprint(w, `{"status":"success","data":{"resultType":"streams","result":[
			{"stream":{"namespace_name":"kube-system","pod_name":"calico-node-x","container_name":"calico-node"},
			 "values":[["1700000000000000000","calico says hi"]]}]}}`)
	default:
		http.NotFound(w, nil)
	}
}

func (f *fakePlutono) set(valiID int, user, pass string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.valiID = valiID
	f.user = user
	f.pass = pass
}

func newLogsCacheUnderTest(t *testing.T, valiID int) (*perShootLogs, *fakePlutono, *fakeGardener) {
	t.Helper()
	plutono := &fakePlutono{valiID: valiID, user: "u", pass: "p"}
	srv := httptest.NewServer(plutono.handler(t))
	t.Cleanup(srv.Close)

	g := &fakeGardener{info: &gardener.MonitoringInfo{URL: srv.URL, Username: "u", Password: "p"}}
	return newPerShootLogs(g, slog.New(slog.DiscardHandler)), plutono, g
}

// Spec scenario "Discovery finds Vali behind a non-first id".
func TestPerShootLogs_DiscoversValiAndQueries(t *testing.T) {
	cache, _, g := newLogsCacheUnderTest(t, 2)
	clusterID := uuid.New()

	client, err := cache.clientFor(context.Background(), clusterID)
	require.NoError(t, err)
	assert.Equal(t, logs.BackendLoki, client.Backend())

	entries, err := client.Query(context.Background(), &logs.QueryParams{ClusterID: clusterID.String()})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "calico says hi", entries[0].Message)
	assert.Equal(t, "kube-system", entries[0].Namespace)

	// Discovery + client construction resolved once despite two calls.
	_, err = cache.clientFor(context.Background(), clusterID)
	require.NoError(t, err)
	assert.Equal(t, int32(1), g.calls.Load())
}

// Spec scenario "Credentials rotated": the first query 401s, the cache
// re-reads the monitoring secret and retries exactly once.
func TestPerShootLogs_ReResolvesOn401(t *testing.T) {
	cache, plutono, g := newLogsCacheUnderTest(t, 2)
	clusterID := uuid.New()

	client, err := cache.clientFor(context.Background(), clusterID)
	require.NoError(t, err)

	// Rotate the credentials on both sides; the cached client still holds the
	// old pair, so its next call 401s and triggers re-resolution.
	plutono.set(2, "u", "rotated")
	g.info = &gardener.MonitoringInfo{URL: g.info.URL, Username: "u", Password: "rotated"}

	entries, err := client.Query(context.Background(), &logs.QueryParams{ClusterID: clusterID.String()})
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, int32(2), g.calls.Load(), "expected exactly one re-resolution")
}

// Spec scenario id drift: a Plutono re-provision moves Vali to another id;
// the stale proxy base answers 500 and the handle re-discovers.
func TestPerShootLogs_ReDiscoversOnIDDrift(t *testing.T) {
	cache, plutono, _ := newLogsCacheUnderTest(t, 2)
	clusterID := uuid.New()

	client, err := cache.clientFor(context.Background(), clusterID)
	require.NoError(t, err)

	plutono.set(3, "u", "p")

	entries, err := client.Query(context.Background(), &logs.QueryParams{ClusterID: clusterID.String()})
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

// Spec scenario "Hibernated or provisioning shoot" (no Vali datasource at
// all): discovery exhausts the id range and reports ErrValiNotFound.
func TestPerShootLogs_NoValiDatasource(t *testing.T) {
	// Vali sits outside the probe range: nothing answers.
	cache, _, _ := newLogsCacheUnderTest(t, 99)

	_, err := cache.clientFor(context.Background(), uuid.New())
	require.ErrorIs(t, err, logs.ErrValiNotFound)
}

func TestPerShootLogs_MonitoringSecretMissing(t *testing.T) {
	g := &fakeGardener{err: gardener.ErrNotFound}
	cache := newPerShootLogs(g, slog.New(slog.DiscardHandler))

	_, err := cache.clientFor(context.Background(), uuid.New())
	require.ErrorIs(t, err, gardener.ErrNotFound)
}

func TestPerShootLogs_WrongCredentialsAbortDiscovery(t *testing.T) {
	cache, plutono, _ := newLogsCacheUnderTest(t, 2)
	plutono.set(2, "someone-else", "different")

	_, err := cache.clientFor(context.Background(), uuid.New())
	require.Error(t, err)
	var statusErr *logs.StatusError
	require.ErrorAs(t, err, &statusErr)
	assert.Equal(t, http.StatusUnauthorized, statusErr.StatusCode)
}

func TestPerShootLogs_LabelsThroughProxy(t *testing.T) {
	cache, _, _ := newLogsCacheUnderTest(t, 2)
	clusterID := uuid.New()

	client, err := cache.clientFor(context.Background(), clusterID)
	require.NoError(t, err)

	labels, err := client.Labels(context.Background(), clusterID.String(), "", time.Time{}, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, []string{"kube-system"}, labels.Namespaces)
}
