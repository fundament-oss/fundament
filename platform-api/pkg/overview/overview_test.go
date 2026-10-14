package overview_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/fundament-oss/fundament/platform-api/internal/testpg"
	"github.com/fundament-oss/fundament/platform-api/pkg/overview"
	platformv1 "github.com/fundament-oss/fundament/platform-api/pkg/proto/gen/platform/v1"
	"github.com/fundament-oss/fundament/platform-api/pkg/proto/gen/platform/v1/platformv1connect"
)

const consoleOrigin = "https://console.fundament.localhost:8443"

// newServer serves the overview over a pool connected as the public role, so
// every read goes through its (absent) table grants.
func newServer(t *testing.T, env *testpg.Env, ttl time.Duration) *httptest.Server {
	t.Helper()
	server := overview.New(slog.New(slog.DiscardHandler), env.RolePool(t, "fun_platform_public_api"), overview.Config{
		CORSAllowedOrigins: []string{consoleOrigin},
		CacheTTL:           ttl,
	})
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// getOverview calls the RPC the way the console does: an anonymous HTTP GET.
func getOverview(t *testing.T, ts *httptest.Server) (*platformv1.GetPlatformOverviewResponse, *http.Response, string) {
	t.Helper()

	query := url.Values{"connect": {"v1"}, "encoding": {"json"}, "message": {"{}"}}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
		ts.URL+platformv1connect.OverviewServiceGetPlatformOverviewProcedure+"?"+query.Encode(), nil)
	require.NoError(t, err)

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	out := &platformv1.GetPlatformOverviewResponse{}
	require.NoError(t, protojson.Unmarshal(body, out))
	return out, resp, string(body)
}

func findOrganization(resp *platformv1.GetPlatformOverviewResponse, name string) *platformv1.OrganizationOverview {
	for _, o := range resp.GetOrganizations() {
		if o.GetName() == name {
			return o
		}
	}
	return nil
}

// A deleted row at any level stops counting, and so does everything under it.
func TestOverviewCountsLiveRowsOnly(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	seed := env.Seed(t)

	org := seed.Organization("counted", "Counted Org")
	cluster := seed.Cluster(org, "live")
	project := seed.Project(cluster, "live-project")
	seed.Namespace(project, "ns-live-1")
	seed.Namespace(project, "ns-live-2")
	seed.SoftDelete("namespaces", seed.Namespace(project, "ns-deleted"))

	// Deletes go bottom-up: the verify_deleted triggers refuse a project with
	// live namespaces and a cluster with live projects.
	deletedProject := seed.Project(cluster, "deleted-project")
	seed.SoftDelete("namespaces", seed.Namespace(deletedProject, "ns-under-deleted-project"))
	seed.SoftDelete("projects", deletedProject)

	deletedCluster := seed.Cluster(org, "deleted")
	underDeleted := seed.Project(deletedCluster, "project-under-deleted-cluster")
	seed.SoftDelete("namespaces", seed.Namespace(underDeleted, "ns-under-deleted-cluster"))
	seed.SoftDelete("projects", underDeleted)
	seed.SoftDelete("clusters", deletedCluster)

	// No trigger guards an organization, so a deleted one can still hold live
	// clusters; they must not count either.
	gone := seed.Organization("gone", "Gone Org")
	seed.Cluster(gone, "orphaned")
	seed.SoftDelete("organizations", gone)

	resp, _, _ := getOverview(t, newServer(t, env, time.Minute))

	got := findOrganization(resp, "counted")
	require.NotNil(t, got)
	assert.Equal(t, "Counted Org", got.GetAlias())
	assert.Equal(t, int32(1), got.GetClusters())
	assert.Equal(t, int32(1), got.GetProjects())
	assert.Equal(t, int32(2), got.GetNamespaces())

	assert.Nil(t, findOrganization(resp, "gone"), "a deleted organization is not listed")
	assert.Nil(t, findOrganization(resp, "system"), "the seeded system organization is no tenant")

	var clusters, projects, namespaces int32
	for _, o := range resp.GetOrganizations() {
		clusters += o.GetClusters()
		projects += o.GetProjects()
		namespaces += o.GetNamespaces()
	}
	totals := resp.GetTotals()
	assert.Equal(t, int32(len(resp.GetOrganizations())), totals.GetOrganizations())
	assert.Equal(t, clusters, totals.GetClusters())
	assert.Equal(t, projects, totals.GetProjects())
	assert.Equal(t, namespaces, totals.GetNamespaces())
	assert.NotNil(t, resp.GetGeneratedAt())
}

// The page shows names and counts only: nothing that names a cluster,
// project or namespace, and no ids.
func TestOverviewExposesNoNamesOrIDs(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	seed := env.Seed(t)

	org := seed.Organization("visible-org", "Visible Org")
	cluster := seed.Cluster(org, "secret-cluster-name")
	project := seed.Project(cluster, "secret-project-name")
	namespace := seed.Namespace(project, "secret-namespace-name")

	_, _, body := getOverview(t, newServer(t, env, time.Minute))

	assert.Contains(t, body, "visible-org")
	for _, secret := range []string{
		"secret-cluster-name", "secret-project-name", "secret-namespace-name",
		org.String(), cluster.String(), project.String(), namespace.String(),
	} {
		assert.NotContains(t, body, secret)
	}
}

// Within the TTL every caller gets the same read, and browsers and proxies
// are told they may cache it for what is left of the TTL.
func TestOverviewIsCached(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	seed := env.Seed(t)
	org := seed.Organization("cached", "Cached Org")
	seed.Cluster(org, "first")

	ts := newServer(t, env, time.Minute)
	first, resp, _ := getOverview(t, ts)
	assert.Equal(t, "public, max-age=60", resp.Header.Get("Cache-Control"))

	seed.Cluster(org, "second")
	second, _, _ := getOverview(t, ts)
	assert.Equal(t, int32(1), findOrganization(second, "cached").GetClusters(), "served from the cache")
	assert.Equal(t, first.GetGeneratedAt().AsTime(), second.GetGeneratedAt().AsTime())

	// A cached answer is advertised for what remains of the TTL, not afresh.
	ageing := newServer(t, env, 2*time.Second)
	getOverview(t, ageing)
	time.Sleep(1100 * time.Millisecond)
	_, resp, _ = getOverview(t, ageing)
	assert.Equal(t, "public, max-age=1", resp.Header.Get("Cache-Control"))

	// A server with a short TTL reads again once it has passed.
	short := newServer(t, env, 10*time.Millisecond)
	getOverview(t, short)
	time.Sleep(20 * time.Millisecond)
	fresh, _, _ := getOverview(t, short)
	assert.Equal(t, int32(2), findOrganization(fresh, "cached").GetClusters())
}

// The console calls without credentials, so CORS allows its origin and never
// credentials.
func TestOverviewCORS(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	ts := newServer(t, env, time.Minute)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodOptions,
		ts.URL+platformv1connect.OverviewServiceGetPlatformOverviewProcedure, nil)
	require.NoError(t, err)
	req.Header.Set("Origin", consoleOrigin)
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, consoleOrigin, resp.Header.Get("Access-Control-Allow-Origin"))
	assert.Empty(t, resp.Header.Get("Access-Control-Allow-Credentials"))
}
