package organization_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/organization-api/pkg/gardener"
	"github.com/fundament-oss/fundament/organization-api/pkg/logs"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

type logsEnv struct {
	env       *testEnv
	token     string
	orgID     uuid.UUID
	clusterID uuid.UUID
	client    organizationv1connect.LogsServiceClient
}

func newLogsEnv(t *testing.T) *logsEnv {
	t.Helper()

	orgID := uuid.New()
	userID := uuid.New()
	env := newTestAPI(t,
		WithOrganization(orgID, "logs-org"),
		WithUser(&UserArgs{ID: userID, Name: "logs-user", OrgIDs: []uuid.UUID{orgID}}),
		WithMockLogs(logs.NewMockClient()),
	)
	token := env.createAuthnToken(t, userID)

	clusterClient := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)
	req := organizationv1.CreateClusterRequest_builder{
		Name:              "logs-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build()
	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())
	res, err := clusterClient.CreateCluster(ctx, req)
	require.NoError(t, err)

	return &logsEnv{
		env:       env,
		token:     token,
		orgID:     orgID,
		clusterID: uuid.MustParse(res.GetClusterId()),
		client:    organizationv1connect.NewLogsServiceClient(env.server.Client(), env.server.URL),
	}
}

func (l *logsEnv) authed(h http.Header) {
	h.Set("Authorization", "Bearer "+l.token)
	h.Set("Fun-Organization", l.orgID.String())
}

// Spec scenario "Mock mode renders": QueryLogs against the mock backend
// returns populated entries marked as the full-featured backend.
func Test_Logs_Query_MockBackend(t *testing.T) {
	t.Parallel()
	l := newLogsEnv(t)

	req := organizationv1.QueryLogsRequest_builder{
		ClusterId: l.clusterID.String(),
		Limit:     25,
	}.Build()
	ctx, callInfo := connect.NewClientContext(context.Background())
	l.authed(callInfo.RequestHeader())

	res, err := l.client.QueryLogs(ctx, req)
	require.NoError(t, err)

	entries := res.GetEntries()
	require.Len(t, entries, 25)
	assert.Equal(t, organizationv1.LogBackend_LOG_BACKEND_LOKI, res.GetBackend())
	// Newest first.
	first := entries[0].GetTimestamp().AsTime()
	second := entries[1].GetTimestamp().AsTime()
	assert.True(t, first.After(second))
	for _, e := range entries {
		assert.NotEmpty(t, e.GetMessage())
		assert.NotEmpty(t, e.GetNamespace())
	}
}

func Test_Logs_Query_NamespaceFilter(t *testing.T) {
	t.Parallel()
	l := newLogsEnv(t)

	req := organizationv1.QueryLogsRequest_builder{
		ClusterId: l.clusterID.String(),
		Namespace: "kube-system",
		Limit:     10,
	}.Build()
	ctx, callInfo := connect.NewClientContext(context.Background())
	l.authed(callInfo.RequestHeader())

	res, err := l.client.QueryLogs(ctx, req)
	require.NoError(t, err)
	require.NotEmpty(t, res.GetEntries())
	for _, e := range res.GetEntries() {
		assert.Equal(t, "kube-system", e.GetNamespace())
	}
}

// Spec scenario "Unknown cluster": querying a cluster id that does not exist
// fails with not-found instead of silently returning empty results.
func Test_Logs_Query_UnknownCluster_NotFound(t *testing.T) {
	t.Parallel()
	l := newLogsEnv(t)

	req := organizationv1.QueryLogsRequest_builder{
		ClusterId: uuid.NewString(),
	}.Build()
	ctx, callInfo := connect.NewClientContext(context.Background())
	l.authed(callInfo.RequestHeader())

	_, err := l.client.QueryLogs(ctx, req)
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func Test_Logs_GetLogLabels_MockBackend(t *testing.T) {
	t.Parallel()
	l := newLogsEnv(t)

	req := organizationv1.GetLogLabelsRequest_builder{
		ClusterId: l.clusterID.String(),
	}.Build()
	ctx, callInfo := connect.NewClientContext(context.Background())
	l.authed(callInfo.RequestHeader())

	res, err := l.client.GetLogLabels(ctx, req)
	require.NoError(t, err)
	assert.Contains(t, res.GetNamespaces(), "kube-system")
	assert.NotEmpty(t, res.GetPods())
	assert.Equal(t, organizationv1.LogBackend_LOG_BACKEND_LOKI, res.GetBackend())
}

// Regression: GetLogLabels verifies the cluster exists (the original branch
// skipped this check).
func Test_Logs_GetLogLabels_UnknownCluster_NotFound(t *testing.T) {
	t.Parallel()
	l := newLogsEnv(t)

	req := organizationv1.GetLogLabelsRequest_builder{
		ClusterId: uuid.NewString(),
	}.Build()
	ctx, callInfo := connect.NewClientContext(context.Background())
	l.authed(callInfo.RequestHeader())

	_, err := l.client.GetLogLabels(ctx, req)
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

// plutonoFake simulates a shoot Plutono behind the seed ingress basic auth
// for handler-level tests: proxy by numeric id only, Vali behind id 2 with
// fixed labels/streams, everything else admin-only or unable-to-load.
type plutonoFake struct{}

func (plutonoFake) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := r.BasicAuth(); !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/datasources/proxy/2/vali/api/v1/label/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":["kube-system"]}`))
		case r.URL.Path == "/api/datasources/proxy/2/vali/api/v1/query_range":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[
				{"stream":{"namespace_name":"kube-system","pod_name":"calico-node-x","container_name":"calico-node"},
				 "values":[["1700000000000000000","calico says hi"]]}]}}`))
		case strings.HasPrefix(r.URL.Path, "/api/datasources/proxy/"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"Unable to load datasource meta data"}`))
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	})
}

func newPerShootLogsEnv(t *testing.T, g gardener.Client, extra ...APIOption) *logsEnv {
	t.Helper()

	orgID := uuid.New()
	userID := uuid.New()
	opts := make([]APIOption, 0, 3+len(extra))
	opts = append(opts,
		WithOrganization(orgID, "logs-org"),
		WithUser(&UserArgs{ID: userID, Name: "logs-user", OrgIDs: []uuid.UUID{orgID}}),
		WithLogsBackend("per-shoot", g),
	)
	opts = append(opts, extra...)
	env := newTestAPI(t, opts...)
	token := env.createAuthnToken(t, userID)

	clusterClient := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)
	req := organizationv1.CreateClusterRequest_builder{
		Name:              "logs-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build()
	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())
	res, err := clusterClient.CreateCluster(ctx, req)
	require.NoError(t, err)

	return &logsEnv{
		env:       env,
		token:     token,
		orgID:     orgID,
		clusterID: uuid.MustParse(res.GetClusterId()),
		client:    organizationv1connect.NewLogsServiceClient(env.server.Client(), env.server.URL),
	}
}

// Spec scenario "Hibernated or provisioning shoot": a missing monitoring
// secret degrades to empty responses marked LOG_BACKEND_NONE — never a
// connect error.
func Test_Logs_PerShoot_MonitoringMissing_Degrades(t *testing.T) {
	t.Parallel()
	// An empty mapGardener answers ErrNotFound for every cluster.
	l := newPerShootLogsEnv(t, &mapGardener{info: make(map[uuid.UUID]*gardener.MonitoringInfo)})

	qreq := organizationv1.QueryLogsRequest_builder{
		ClusterId: l.clusterID.String(),
	}.Build()
	ctx, callInfo := connect.NewClientContext(context.Background())
	l.authed(callInfo.RequestHeader())
	qres, err := l.client.QueryLogs(ctx, qreq)
	require.NoError(t, err, "a cluster without logs must not fail the RPC")
	assert.Empty(t, qres.GetEntries())
	assert.Equal(t, organizationv1.LogBackend_LOG_BACKEND_NONE, qres.GetBackend())

	lreq := organizationv1.GetLogLabelsRequest_builder{
		ClusterId: l.clusterID.String(),
	}.Build()
	lres, err := l.client.GetLogLabels(ctx, lreq)
	require.NoError(t, err)
	assert.Empty(t, lres.GetNamespaces())
	assert.Equal(t, organizationv1.LogBackend_LOG_BACKEND_NONE, lres.GetBackend())
}

// Spec scenario dead backend: the monitoring secret resolves but the seed
// ingress is unreachable — still empty + LOG_BACKEND_NONE, no connect error.
func Test_Logs_PerShoot_DeadBackend_Degrades(t *testing.T) {
	t.Parallel()

	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	g := &mapGardener{info: make(map[uuid.UUID]*gardener.MonitoringInfo)}
	l := newPerShootLogsEnv(t, g)
	g.set(l.clusterID, &gardener.MonitoringInfo{URL: closed.URL, Username: "u", Password: "p"})

	req := organizationv1.QueryLogsRequest_builder{
		ClusterId: l.clusterID.String(),
	}.Build()
	ctx, callInfo := connect.NewClientContext(context.Background())
	l.authed(callInfo.RequestHeader())

	res, err := l.client.QueryLogs(ctx, req)
	require.NoError(t, err, "an unreachable backend must not fail the RPC")
	assert.Empty(t, res.GetEntries())
	assert.Equal(t, organizationv1.LogBackend_LOG_BACKEND_NONE, res.GetBackend())
}

// Spec scenario "Plugin pod logs": a query pinned to a namespace+pod that
// Vali does not cover reads through the kube-api-proxy with the caller's
// token and reports LOG_BACKEND_KUBERNETES.
func Test_Logs_PluginPod_RoutesThroughKubeProxy(t *testing.T) {
	t.Parallel()

	plutonoSrv := httptest.NewServer(plutonoFake{}.handler())
	t.Cleanup(plutonoSrv.Close)
	g := &mapGardener{info: make(map[uuid.UUID]*gardener.MonitoringInfo)}

	var gotPath, gotAuth string
	kube := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("2026-08-05T12:00:00.000000000Z plugin says hi\n"))
	}))
	t.Cleanup(kube.Close)

	l := newPerShootLogsEnv(t, g, WithKubeAPIProxy(kube.URL))
	g.set(l.clusterID, &gardener.MonitoringInfo{URL: plutonoSrv.URL, Username: "u", Password: "p"})

	req := organizationv1.QueryLogsRequest_builder{
		ClusterId: l.clusterID.String(),
		Namespace: "plugin-envoy-gateway",
		Pod:       "envoy-gateway-abc",
	}.Build()
	ctx, callInfo := connect.NewClientContext(context.Background())
	l.authed(callInfo.RequestHeader())

	res, err := l.client.QueryLogs(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.GetEntries(), 1)
	assert.Equal(t, "plugin says hi", res.GetEntries()[0].GetMessage())
	assert.Equal(t, organizationv1.LogBackend_LOG_BACKEND_KUBERNETES, res.GetBackend())
	assert.Contains(t, gotPath, "/namespaces/plugin-envoy-gateway/pods/envoy-gateway-abc/log")
	assert.Equal(t, "Bearer "+l.token, gotAuth)
}

// A namespace Vali does cover stays on the Vali backend even when a pod is
// selected.
func Test_Logs_SystemPod_StaysOnVali(t *testing.T) {
	t.Parallel()

	plutonoSrv := httptest.NewServer(plutonoFake{}.handler())
	t.Cleanup(plutonoSrv.Close)
	g := &mapGardener{info: make(map[uuid.UUID]*gardener.MonitoringInfo)}

	kube := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(kube.Close)

	l := newPerShootLogsEnv(t, g, WithKubeAPIProxy(kube.URL))
	g.set(l.clusterID, &gardener.MonitoringInfo{URL: plutonoSrv.URL, Username: "u", Password: "p"})

	req := organizationv1.QueryLogsRequest_builder{
		ClusterId: l.clusterID.String(),
		Namespace: "kube-system",
		Pod:       "calico-node-x",
	}.Build()
	ctx, callInfo := connect.NewClientContext(context.Background())
	l.authed(callInfo.RequestHeader())

	res, err := l.client.QueryLogs(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, organizationv1.LogBackend_LOG_BACKEND_LOKI, res.GetBackend())
	require.Len(t, res.GetEntries(), 1)
	assert.Equal(t, "calico says hi", res.GetEntries()[0].GetMessage())
}
