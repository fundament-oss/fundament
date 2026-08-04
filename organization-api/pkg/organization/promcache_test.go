package organization

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/organization-api/pkg/gardener"
	prom "github.com/fundament-oss/fundament/organization-api/pkg/prometheus"
)

type fakeGardener struct {
	info  *gardener.MonitoringInfo
	err   error
	calls atomic.Int32
}

func (f *fakeGardener) Monitoring(context.Context, uuid.UUID) (*gardener.MonitoringInfo, error) {
	f.calls.Add(1)
	if f.err != nil {
		return nil, f.err
	}
	return f.info, nil
}

// fakeProm counts queries and fails with the configured error until reset.
type fakeProm struct {
	queryErr error
	queries  atomic.Int32
}

func (f *fakeProm) Query(context.Context, string, time.Time) ([]prom.Sample, error) {
	f.queries.Add(1)
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	return []prom.Sample{{Value: 42}}, nil
}

func (f *fakeProm) QueryRange(context.Context, string, time.Time, time.Time, time.Duration) ([]prom.TimeSeries, error) {
	f.queries.Add(1)
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	return []prom.TimeSeries{{Samples: []prom.DataPoint{{Value: 42}}}}, nil
}

func testCache(g gardener.Client) *perShootClients {
	return newPerShootClients(g, slog.New(slog.DiscardHandler))
}

func TestPerShootClients_ResolvesAndCaches(t *testing.T) {
	g := &fakeGardener{info: &gardener.MonitoringInfo{URL: "https://plutono", PrometheusURL: "https://prom", Username: "u", Password: "p"}}
	cache := testCache(g)

	var built atomic.Int32
	cache.newClient = func(base, _, _ string) prom.Client {
		built.Add(1)
		assert.Equal(t, "https://prom", base)
		return &fakeProm{}
	}

	clusterID := uuid.New()
	client, err := cache.clientFor(context.Background(), clusterID)
	require.NoError(t, err)

	_, err = client.Query(context.Background(), "up", time.Now())
	require.NoError(t, err)
	_, err = client.Query(context.Background(), "up", time.Now())
	require.NoError(t, err)

	assert.Equal(t, int32(1), built.Load(), "second query must reuse the cached client")
	assert.Equal(t, int32(1), g.calls.Load())
}

func TestPerShootClients_ExpiryReResolves(t *testing.T) {
	g := &fakeGardener{info: &gardener.MonitoringInfo{URL: "https://plutono", PrometheusURL: "https://prom", Username: "u", Password: "p"}}
	cache := testCache(g)
	cache.newClient = func(string, string, string) prom.Client { return &fakeProm{} }

	current := time.Now()
	cache.now = func() time.Time { return current }

	clusterID := uuid.New()
	_, err := cache.resolved(context.Background(), clusterID)
	require.NoError(t, err)

	// Just past the 70%-of-TTL expiry, pinning the early-refresh factor.
	current = current.Add(perShootTTL*7/10 + time.Second)
	_, err = cache.resolved(context.Background(), clusterID)
	require.NoError(t, err)
	assert.Equal(t, int32(2), g.calls.Load())
}

func TestPerShootClients_NotFoundPassthrough(t *testing.T) {
	cache := testCache(&fakeGardener{err: gardener.ErrNotFound})

	_, err := cache.clientFor(context.Background(), uuid.New())
	require.ErrorIs(t, err, gardener.ErrNotFound)
}

func TestPerShootClient_RetriesOnceOn401(t *testing.T) {
	g := &fakeGardener{info: &gardener.MonitoringInfo{URL: "https://plutono", PrometheusURL: "https://prom", Username: "u", Password: "old"}}
	cache := testCache(g)

	stale := &fakeProm{queryErr: &prom.StatusError{StatusCode: http.StatusUnauthorized}}
	fresh := &fakeProm{}
	clients := []prom.Client{stale, fresh}
	var idx atomic.Int32
	cache.newClient = func(string, string, string) prom.Client {
		return clients[idx.Add(1)-1]
	}

	client, err := cache.clientFor(context.Background(), uuid.New())
	require.NoError(t, err)

	samples, err := client.Query(context.Background(), "up", time.Now())
	require.NoError(t, err)
	require.Len(t, samples, 1)
	assert.Equal(t, float64(42), samples[0].Value)
	assert.Equal(t, int32(1), stale.queries.Load())
	assert.Equal(t, int32(1), fresh.queries.Load())
	assert.Equal(t, int32(2), g.calls.Load(), "401 must trigger exactly one re-resolution")
}

// QueryRange goes through the same 401 retry as Query (shared helper), but
// it is what every time-series RPC uses — pin it separately.
func TestPerShootClient_QueryRangeRetriesOnceOn401(t *testing.T) {
	g := &fakeGardener{info: &gardener.MonitoringInfo{URL: "https://plutono", PrometheusURL: "https://prom", Username: "u", Password: "old"}}
	cache := testCache(g)

	stale := &fakeProm{queryErr: &prom.StatusError{StatusCode: http.StatusUnauthorized}}
	fresh := &fakeProm{}
	clients := []prom.Client{stale, fresh}
	var idx atomic.Int32
	cache.newClient = func(string, string, string) prom.Client {
		return clients[idx.Add(1)-1]
	}

	client, err := cache.clientFor(context.Background(), uuid.New())
	require.NoError(t, err)

	now := time.Now()
	series, err := client.QueryRange(context.Background(), "up", now.Add(-time.Hour), now, time.Minute)
	require.NoError(t, err)
	require.Len(t, series, 1)
	assert.Equal(t, int32(1), stale.queries.Load())
	assert.Equal(t, int32(1), fresh.queries.Load())
	assert.Equal(t, int32(2), g.calls.Load(), "401 must trigger exactly one re-resolution")
}

func TestPerShootClient_Non401NotRetried(t *testing.T) {
	g := &fakeGardener{info: &gardener.MonitoringInfo{URL: "https://plutono", PrometheusURL: "https://prom", Username: "u", Password: "p"}}
	cache := testCache(g)

	failing := &fakeProm{queryErr: &prom.StatusError{StatusCode: http.StatusBadGateway}}
	cache.newClient = func(string, string, string) prom.Client { return failing }

	client, err := cache.clientFor(context.Background(), uuid.New())
	require.NoError(t, err)

	_, err = client.Query(context.Background(), "up", time.Now())
	require.Error(t, err)
	assert.Equal(t, int32(1), failing.queries.Load())
	assert.Equal(t, int32(1), g.calls.Load())
}

// ---- promClientFor tri-state selection ----

func triStateServer(url string, g gardener.Client) *Server {
	logger := slog.New(slog.DiscardHandler)
	return &Server{
		logger:        logger,
		prometheusURL: url,
		gardener:      g,
		perShoot:      newPerShootClients(g, logger),
	}
}

func TestPromClientFor_MockModeUsesStubWithoutMockClient(t *testing.T) {
	s := triStateServer("mock", gardener.NoopClient{})

	client, err := s.promClientFor(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.IsType(t, prom.StubClient{}, client)
}

// Set-but-empty PROMETHEUS_URL collapses to the "mock" default at the env
// layer; the server treats "" the same so both layers agree.
func TestPromClientFor_EmptyBehavesAsMock(t *testing.T) {
	s := triStateServer("", gardener.NoopClient{})

	client, err := s.promClientFor(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.IsType(t, prom.StubClient{}, client)
}

func TestPromClientFor_URLUsesGlobalHTTPClient(t *testing.T) {
	s := triStateServer("http://prometheus.example", gardener.NoopClient{})

	client, err := s.promClientFor(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.IsType(t, &prom.HTTPClient{}, client)
}

func TestPromClientFor_PerShootWithoutGardenerIsNotFound(t *testing.T) {
	s := triStateServer("per-shoot", gardener.NoopClient{})

	_, err := s.promClientFor(context.Background(), uuid.New())
	require.ErrorIs(t, err, gardener.ErrNotFound)
}

func TestPromClientFor_PerShootResolves(t *testing.T) {
	g := &fakeGardener{info: &gardener.MonitoringInfo{URL: "https://plutono", PrometheusURL: "https://prom", Username: "u", Password: "p"}}
	s := triStateServer("per-shoot", g)
	s.perShoot.newClient = func(string, string, string) prom.Client { return &fakeProm{} }

	client, err := s.promClientFor(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.IsType(t, &perShootClient{}, client)
}

func TestPerShootClients_MissingPrometheusURLIsNotFound(t *testing.T) {
	// A Gardener version that does not annotate prometheus-url must degrade
	// like a missing monitoring stack, not fail hard.
	g := &fakeGardener{info: &gardener.MonitoringInfo{URL: "https://plutono", Username: "u", Password: "p"}}
	cache := testCache(g)

	_, err := cache.clientFor(context.Background(), uuid.New())
	require.ErrorIs(t, err, gardener.ErrNotFound)
}
