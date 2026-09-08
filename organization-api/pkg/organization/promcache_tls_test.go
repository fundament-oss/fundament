package organization

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/organization-api/pkg/catrust"
	"github.com/fundament-oss/fundament/organization-api/pkg/catrust/certtest"
	"github.com/fundament-oss/fundament/organization-api/pkg/gardener"
)

// perClusterGardener serves a different MonitoringInfo per cluster and counts
// resolutions.
type perClusterGardener struct {
	mu    sync.Mutex
	info  map[uuid.UUID]*gardener.MonitoringInfo
	calls int
}

func (p *perClusterGardener) set(id uuid.UUID, info *gardener.MonitoringInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.info[id] = info
}

func (p *perClusterGardener) Monitoring(_ context.Context, id uuid.UUID) (*gardener.MonitoringInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	info, ok := p.info[id]
	if !ok {
		return nil, gardener.ErrNotFound
	}
	return info, nil
}

// shootPrometheus starts a TLS Prometheus stand-in behind its own shoot CA.
func shootPrometheus(t *testing.T) (url string, ca *certtest.CA) {
	t.Helper()

	ca = certtest.NewCA(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/query", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700000000,"1"]}]}}`)
	})

	srv := httptest.NewUnstartedServer(mux)
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{ca.ServerCert(t)}, MinVersion: tls.VersionTLS12}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	return srv.URL, ca
}

func monitoringAt(url string, ca *certtest.CA) *gardener.MonitoringInfo {
	info := &gardener.MonitoringInfo{URL: url, PrometheusURL: url, Username: "u", Password: "p"}
	if ca != nil {
		info.CABundle = ca.PEM
	}
	return info
}

func trustWithoutBundle(t *testing.T) *catrust.Trust {
	t.Helper()
	trust, err := catrust.New("")
	require.NoError(t, err)
	return trust
}

// Two clusters need two different anchors at the same time — what one
// process-wide transport cannot hold.
func TestPerShootClients_TrustsEachShootsOwnCA(t *testing.T) {
	urlA, caA := shootPrometheus(t)
	urlB, caB := shootPrometheus(t)

	idA, idB := uuid.New(), uuid.New()
	g := &perClusterGardener{info: map[uuid.UUID]*gardener.MonitoringInfo{
		idA: monitoringAt(urlA, caA),
		idB: monitoringAt(urlB, caB),
	}}
	cache := newPerShootClients(g, slog.New(slog.DiscardHandler), trustWithoutBundle(t))

	for name, id := range map[string]uuid.UUID{"shoot A": idA, "shoot B": idB} {
		client, err := cache.clientFor(context.Background(), id)
		require.NoError(t, err, name)

		samples, err := client.Query(context.Background(), "up", time.Now())
		require.NoError(t, err, "%s must verify against its own CA", name)
		assert.Len(t, samples, 1, name)
	}
}

// Another shoot's CA must not verify, even though every shoot CA is CN=ca.
func TestPerShootClients_RejectsAnotherShootsCA(t *testing.T) {
	urlA, _ := shootPrometheus(t)
	_, caB := shootPrometheus(t)

	id := uuid.New()
	g := &perClusterGardener{info: map[uuid.UUID]*gardener.MonitoringInfo{id: monitoringAt(urlA, caB)}}
	cache := newPerShootClients(g, slog.New(slog.DiscardHandler), trustWithoutBundle(t))

	client, err := cache.clientFor(context.Background(), id)
	require.NoError(t, err)

	_, err = client.Query(context.Background(), "up", time.Now())
	require.Error(t, err)
	var unknownAuthority x509.UnknownAuthorityError
	assert.ErrorAs(t, err, &unknownAuthority)
}

// A rotated CA re-resolves once instead of serving a dead transport until the
// TTL expires.
func TestPerShootClients_ReResolvesOnTLSVerificationFailure(t *testing.T) {
	urlA, caA := shootPrometheus(t)
	_, caStale := shootPrometheus(t)

	id := uuid.New()
	g := &perClusterGardener{info: map[uuid.UUID]*gardener.MonitoringInfo{id: monitoringAt(urlA, caStale)}}
	cache := newPerShootClients(g, slog.New(slog.DiscardHandler), trustWithoutBundle(t))

	client, err := cache.clientFor(context.Background(), id)
	require.NoError(t, err)

	g.set(id, monitoringAt(urlA, caA)) // rotation lands

	samples, err := client.Query(context.Background(), "up", time.Now())
	require.NoError(t, err, "the stale entry must be re-resolved, not served until expiry")
	assert.Len(t, samples, 1)
	assert.Equal(t, 2, g.calls, "exactly one re-resolution")
}

// A CA that never verifies must not re-resolve on every query: resolution
// succeeds, so only the retry rate limit stands between a stable
// misconfiguration and a Gardener call per query.
func TestPerShootClients_PersistentTLSFailureReResolvesOncePerWindow(t *testing.T) {
	urlA, _ := shootPrometheus(t)
	_, caWrong := shootPrometheus(t)

	id := uuid.New()
	g := &perClusterGardener{info: map[uuid.UUID]*gardener.MonitoringInfo{id: monitoringAt(urlA, caWrong)}}
	cache := newPerShootClients(g, slog.New(slog.DiscardHandler), trustWithoutBundle(t))
	current := time.Now()
	cache.now = func() time.Time { return current }

	client, err := cache.clientFor(context.Background(), id)
	require.NoError(t, err)

	for range 3 {
		_, err = client.Query(context.Background(), "up", time.Now())
		require.Error(t, err)
	}
	assert.Equal(t, 2, g.calls, "one re-resolution, then rate-limited")

	current = current.Add(negativeTTL + time.Second)
	_, err = client.Query(context.Background(), "up", time.Now())
	require.Error(t, err)
	assert.Equal(t, 3, g.calls, "the window passed, one more attempt")
}

// No CA published (wildcard-cert landscapes) still resolves; only the
// connection can fail.
func TestPerShootClients_NoCADegradesToPlatformRoots(t *testing.T) {
	urlA, _ := shootPrometheus(t)

	id := uuid.New()
	g := &perClusterGardener{info: map[uuid.UUID]*gardener.MonitoringInfo{id: monitoringAt(urlA, nil)}}
	cache := newPerShootClients(g, slog.New(slog.DiscardHandler), trustWithoutBundle(t))

	client, err := cache.clientFor(context.Background(), id)
	require.NoError(t, err)

	_, err = client.Query(context.Background(), "up", time.Now())
	require.Error(t, err)
}
