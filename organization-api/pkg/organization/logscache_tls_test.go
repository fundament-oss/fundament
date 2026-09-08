package organization

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"log/slog"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/organization-api/pkg/catrust/certtest"
	"github.com/fundament-oss/fundament/organization-api/pkg/gardener"
	"github.com/fundament-oss/fundament/organization-api/pkg/logs"
)

// tlsPlutono is a fakePlutono behind a shoot CA that can be rotated at runtime.
type tlsPlutono struct {
	url  string
	srv  *httptest.Server
	cert atomic.Pointer[tls.Certificate]
}

func newTLSPlutono(t *testing.T, ca *certtest.CA) *tlsPlutono {
	t.Helper()
	p := &tlsPlutono{}
	p.rotate(t, ca)

	plutono := &fakePlutono{valiID: 2, user: "u", pass: "p"}
	srv := httptest.NewUnstartedServer(plutono.handler(t))
	srv.TLS = &tls.Config{
		MinVersion: tls.VersionTLS12,
		// GetConfigForClient wins over Certificates, which httptest fills in.
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			return &tls.Config{Certificates: []tls.Certificate{*p.cert.Load()}, MinVersion: tls.VersionTLS12}, nil
		},
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	p.url = srv.URL
	p.srv = srv
	return p
}

// rotate installs a certificate from ca and drops open connections, so the
// next request handshakes against it (as an ingress restart would).
func (p *tlsPlutono) rotate(t *testing.T, ca *certtest.CA) {
	t.Helper()
	cert := ca.ServerCert(t)
	p.cert.Store(&cert)
	if p.srv != nil {
		p.srv.CloseClientConnections()
	}
}

func logsMonitoringAt(url string, ca *certtest.CA) *gardener.MonitoringInfo {
	return &gardener.MonitoringInfo{URL: url, Username: "u", Password: "p", CABundle: ca.PEM}
}

// The datasource probe and the client must both trust the shoot CA, or
// discovery fails before a client exists.
func TestPerShootLogs_TrustsShootCA(t *testing.T) {
	ca := certtest.NewCA(t)
	plutono := newTLSPlutono(t, ca)

	id := uuid.New()
	g := &perClusterGardener{info: map[uuid.UUID]*gardener.MonitoringInfo{id: logsMonitoringAt(plutono.url, ca)}}
	cache := newPerShootLogs(g, slog.New(slog.DiscardHandler), trustWithoutBundle(t))

	client, err := cache.clientFor(context.Background(), id)
	require.NoError(t, err)
	entries, err := client.Query(context.Background(), &logs.QueryParams{ClusterID: id.String()})
	require.NoError(t, err)
	assert.Len(t, entries, 1)

	other := uuid.New()
	g.set(other, logsMonitoringAt(plutono.url, certtest.NewCA(t)))
	_, err = cache.clientFor(context.Background(), other)
	require.Error(t, err)
	var unknownAuthority x509.UnknownAuthorityError
	assert.ErrorAs(t, err, &unknownAuthority)
}

// A CA rotation mid-TTL re-resolves once on the logs path too.
func TestPerShootLogs_ReResolvesOnRotatedCA(t *testing.T) {
	old, rotated := certtest.NewCA(t), certtest.NewCA(t)
	plutono := newTLSPlutono(t, old)

	id := uuid.New()
	g := &perClusterGardener{info: map[uuid.UUID]*gardener.MonitoringInfo{id: logsMonitoringAt(plutono.url, old)}}
	cache := newPerShootLogs(g, slog.New(slog.DiscardHandler), trustWithoutBundle(t))

	client, err := cache.clientFor(context.Background(), id)
	require.NoError(t, err)

	plutono.rotate(t, rotated)
	g.set(id, logsMonitoringAt(plutono.url, rotated))

	entries, err := client.Query(context.Background(), &logs.QueryParams{ClusterID: id.String()})
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, 2, g.calls)
}
