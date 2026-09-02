package organization

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/organization-api/pkg/catrust"
	"github.com/fundament-oss/fundament/organization-api/pkg/gardener"
	prom "github.com/fundament-oss/fundament/organization-api/pkg/prometheus"
)

// perShootClients resolves and caches one Prometheus client per cluster,
// targeting the shoot's Prometheus ingress (the "prometheus-url" annotation
// on the Gardener monitoring secret) with the secret's basic-auth
// credentials. Verified live 2026-08-02: the ingress serves the standard
// Prometheus HTTP API with the same auth; Plutono's datasource-lookup APIs
// are admin-only behind the ingress and unusable for discovery.
type perShootClients struct {
	gardener gardener.Client
	logger   *slog.Logger
	now      func() time.Time
	trust    *catrust.Trust // builds the transport from the shoot's own CA

	// newClient is a seam for tests; production wiring uses
	// prom.NewHTTPClientWithAuth.
	newClient func(base, username, password string, transport http.RoundTripper) prom.Client

	cache *perShootCache[prom.Client]
}

func newPerShootClients(g gardener.Client, logger *slog.Logger, trust *catrust.Trust) *perShootClients {
	c := &perShootClients{
		gardener: g,
		logger:   logger,
		now:      time.Now,
		trust:    trust,
		newClient: func(base, username, password string, transport http.RoundTripper) prom.Client {
			return prom.NewHTTPClientWithAuth(base, username, password, prom.WithTransport(transport))
		},
	}
	// Indirections keep c.now / c.newClient assignable by tests after
	// construction.
	c.cache = newPerShootCache("per-shoot prometheus", logger, func() time.Time { return c.now() }, c.resolveClient, defaultResolveTimeout)
	return c
}

func (c *perShootClients) resolveClient(ctx context.Context, clusterID uuid.UUID) (prom.Client, error) {
	info, err := c.gardener.Monitoring(ctx, clusterID)
	if err != nil {
		return nil, fmt.Errorf("look up shoot monitoring: %w", err)
	}
	if info.PrometheusURL == "" {
		return nil, fmt.Errorf("monitoring secret has no prometheus-url annotation: %w", gardener.ErrNotFound)
	}
	return c.newClient(info.PrometheusURL, info.Username, info.Password, c.trust.TransportFor(info.CABundle)), nil
}

// clientFor returns a client for the cluster's per-shoot Prometheus. It
// returns gardener.ErrNotFound (possibly wrapped) while the shoot or its
// monitoring stack is not available yet; callers treat that as "no metrics",
// not as a hard error. The returned client re-resolves once on 401 or TLS
// verification failure, so credential and CA rotation heal without a restart.
func (c *perShootClients) clientFor(ctx context.Context, clusterID uuid.UUID) (prom.Client, error) {
	_, err := c.resolved(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	return &perShootClient{cache: c, clusterID: clusterID}, nil
}

// resolved returns the cached inner client for clusterID, resolving it when
// absent or expired.
func (c *perShootClients) resolved(ctx context.Context, clusterID uuid.UUID) (prom.Client, error) {
	return c.cache.get(ctx, clusterID)
}

// perShootClient is the stable handle handed to query code. Each call fetches
// the currently cached inner client; on a stale-resolution error it
// invalidates, re-resolves, and retries exactly once.
type perShootClient struct {
	cache     *perShootClients
	clusterID uuid.UUID
}

func (p *perShootClient) Query(ctx context.Context, query string, t time.Time) ([]prom.Sample, error) {
	return callWithReResolveOnce(ctx, p.cache.cache, p.clusterID, isStalePromResolution, func(inner prom.Client) ([]prom.Sample, error) {
		return inner.Query(ctx, query, t)
	})
}

func (p *perShootClient) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]prom.TimeSeries, error) {
	return callWithReResolveOnce(ctx, p.cache.cache, p.clusterID, isStalePromResolution, func(inner prom.Client) ([]prom.TimeSeries, error) {
		return inner.QueryRange(ctx, query, start, end, step)
	})
}

// isStalePromResolution: 401 (rotated credentials) or a TLS verification
// failure (rotated shoot CA).
func isStalePromResolution(err error) bool {
	return isUnauthorized(err) || isTLSVerificationFailure(err)
}

func isUnauthorized(err error) bool {
	var statusErr *prom.StatusError
	if !errors.As(err, &statusErr) {
		return false
	}
	return statusErr.StatusCode == http.StatusUnauthorized
}

// isTLSVerificationFailure matches a rotated shoot CA: the cached transport
// carries the CA as of resolution time.
func isTLSVerificationFailure(err error) bool {
	var verificationErr *tls.CertificateVerificationError
	return errors.As(err, &verificationErr)
}
