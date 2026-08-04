package organization_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/organization-api/pkg/gardener"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

// mapGardener serves MonitoringInfo per cluster from a mutable map, so tests
// can register clusters after the server (and thus the client) is built.
type mapGardener struct {
	mu   sync.Mutex
	info map[uuid.UUID]*gardener.MonitoringInfo
}

func (m *mapGardener) set(id uuid.UUID, info *gardener.MonitoringInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.info[id] = info
}

func (m *mapGardener) Monitoring(_ context.Context, id uuid.UUID) (*gardener.MonitoringInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	info, ok := m.info[id]
	if !ok {
		return nil, gardener.ErrNotFound
	}
	return info, nil
}

// fakePrometheus answers every instant query with a single sample of the
// given value, mirroring the per-shoot Prometheus ingress contract.
func fakePrometheus(t *testing.T, value float64) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/query", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"namespace":"default","node":"n1"},"value":[1700000000,"%g"]}]}}`, value)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// Spec scenario "One sick cluster": with three clusters on per-shoot backends
// and one of them unreachable, the org view returns real data for two and a
// metrics-unavailable marker for the third instead of failing.
func Test_Metrics_OrgView_DegradesPerCluster(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()
	garden := &mapGardener{info: make(map[uuid.UUID]*gardener.MonitoringInfo)}

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{ID: userID, Name: "test-user", OrgIDs: []uuid.UUID{orgID}}),
		WithPrometheusBackend("per-shoot", garden),
	)
	token := env.createAuthnToken(t, userID)

	clusterClient := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)
	createCluster := func(name string) uuid.UUID {
		req := connect.NewRequest(organizationv1.CreateClusterRequest_builder{
			Name:              name,
			Region:            "eu-west-1",
			KubernetesVersion: "1.28",
		}.Build())
		req.Header().Set("Authorization", "Bearer "+token)
		req.Header().Set("Fun-Organization", orgID.String())
		res, err := clusterClient.CreateCluster(context.Background(), req)
		require.NoError(t, err)
		return uuid.MustParse(res.Msg.GetClusterId())
	}

	healthy1 := createCluster("healthy-1")
	healthy2 := createCluster("healthy-2")
	sick := createCluster("sick")

	garden.set(healthy1, &gardener.MonitoringInfo{URL: "https://plutono", PrometheusURL: fakePrometheus(t, 2).URL, Username: "u", Password: "p"})
	garden.set(healthy2, &gardener.MonitoringInfo{URL: "https://plutono", PrometheusURL: fakePrometheus(t, 3).URL, Username: "u", Password: "p"})
	// The sick cluster's Prometheus is unreachable: point at a closed port.
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	garden.set(sick, &gardener.MonitoringInfo{URL: "https://plutono", PrometheusURL: closed.URL, Username: "u", Password: "p"})

	metricsClient := organizationv1connect.NewMetricsServiceClient(env.server.Client(), env.server.URL)
	req := connect.NewRequest(organizationv1.GetOrgWorkloadMetricsRequest_builder{}.Build())
	req.Header().Set("Authorization", "Bearer "+token)
	req.Header().Set("Fun-Organization", orgID.String())

	res, err := metricsClient.GetOrgWorkloadMetrics(context.Background(), req)
	require.NoError(t, err, "one sick cluster must not fail the org view")

	summaries := res.Msg.GetClusters()
	require.Len(t, summaries, 3)

	byName := make(map[string]*organizationv1.ClusterWorkloadSummary)
	for _, s := range summaries {
		byName[s.GetClusterName()] = s
	}

	require.Contains(t, byName, "sick")
	assert.True(t, byName["sick"].GetMetricsUnavailable())
	assert.Zero(t, byName["sick"].GetCpu().GetUsed())

	assert.False(t, byName["healthy-1"].GetMetricsUnavailable())
	assert.Equal(t, float64(2), byName["healthy-1"].GetCpu().GetUsed())
	assert.False(t, byName["healthy-2"].GetMetricsUnavailable())
	assert.Equal(t, float64(3), byName["healthy-2"].GetCpu().GetUsed())

	// Totals aggregate only the reachable clusters.
	assert.Equal(t, float64(5), res.Msg.GetTotals().GetCpu().GetUsed())
}

// A cluster whose monitoring secret resolves but whose Prometheus does not
// answer (hibernated shoot, ingress 5xx) must degrade to empty metrics at
// cluster level — not a hard error that 500s the page and kills the stream
// on every tick.
func Test_Metrics_ClusterView_DegradesWhenPrometheusUnreachable(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()
	garden := &mapGardener{info: make(map[uuid.UUID]*gardener.MonitoringInfo)}

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{ID: userID, Name: "test-user", OrgIDs: []uuid.UUID{orgID}}),
		WithPrometheusBackend("per-shoot", garden),
	)
	token := env.createAuthnToken(t, userID)

	clusterClient := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)
	createReq := connect.NewRequest(organizationv1.CreateClusterRequest_builder{
		Name:              "sick",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build())
	createReq.Header().Set("Authorization", "Bearer "+token)
	createReq.Header().Set("Fun-Organization", orgID.String())
	createRes, err := clusterClient.CreateCluster(context.Background(), createReq)
	require.NoError(t, err)
	clusterID := createRes.Msg.GetClusterId()

	// The monitoring secret exists, but the Prometheus behind it is gone.
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	garden.set(uuid.MustParse(clusterID), &gardener.MonitoringInfo{URL: "https://plutono", PrometheusURL: closed.URL, Username: "u", Password: "p"})

	metricsClient := organizationv1connect.NewMetricsServiceClient(env.server.Client(), env.server.URL)

	metricsReq := connect.NewRequest(organizationv1.GetClusterWorkloadMetricsRequest_builder{ClusterId: clusterID}.Build())
	metricsReq.Header().Set("Authorization", "Bearer "+token)
	metricsReq.Header().Set("Fun-Organization", orgID.String())
	metricsRes, err := metricsClient.GetClusterWorkloadMetrics(context.Background(), metricsReq)
	require.NoError(t, err, "an unreachable Prometheus must not fail the cluster view")
	assert.Zero(t, metricsRes.Msg.GetTotals().GetCpu().GetUsed())
	assert.Equal(t, "cores", metricsRes.Msg.GetTotals().GetCpu().GetUnit())
	assert.Empty(t, metricsRes.Msg.GetNodes())
	assert.Empty(t, metricsRes.Msg.GetNamespaces())

	tsReq := connect.NewRequest(organizationv1.GetClusterWorkloadTimeSeriesRequest_builder{ClusterId: clusterID}.Build())
	tsReq.Header().Set("Authorization", "Bearer "+token)
	tsReq.Header().Set("Fun-Organization", orgID.String())
	tsRes, err := metricsClient.GetClusterWorkloadTimeSeries(context.Background(), tsReq)
	require.NoError(t, err, "an unreachable Prometheus must not fail the time-series view")
	assert.Empty(t, tsRes.Msg.GetCpuCores())
}
