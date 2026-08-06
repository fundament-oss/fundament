package organization

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

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

	// newClient is a seam for tests; production wiring uses
	// prom.NewHTTPClientWithAuth.
	newClient func(base, username, password string) prom.Client

	cache *perShootCache[prom.Client]
}

func newPerShootClients(g gardener.Client, logger *slog.Logger, opts ...prom.Option) *perShootClients {
	c := &perShootClients{
		gardener: g,
		logger:   logger,
		now:      time.Now,
		newClient: func(base, username, password string) prom.Client {
			return prom.NewHTTPClientWithAuth(base, username, password, opts...)
		},
	}
	// Indirections keep c.now / c.newClient assignable by tests after
	// construction.
	c.cache = newPerShootCache(func() time.Time { return c.now() }, c.resolveClient)
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
	return c.newClient(info.PrometheusURL, info.Username, info.Password), nil
}

// clientFor returns a client for the cluster's per-shoot Prometheus. It
// returns gardener.ErrNotFound (possibly wrapped) while the shoot or its
// monitoring stack is not available yet; callers treat that as "no metrics",
// not as a hard error. The returned client re-resolves once on 401 so
// credential rotation heals without a restart.
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

func (c *perShootClients) invalidate(clusterID uuid.UUID) {
	c.cache.invalidate(clusterID)
}

// perShootClient is the stable handle handed to query code. Each call fetches
// the currently cached inner client; on 401 it invalidates, re-resolves, and
// retries exactly once.
type perShootClient struct {
	cache     *perShootClients
	clusterID uuid.UUID
}

func (p *perShootClient) Query(ctx context.Context, query string, t time.Time) ([]prom.Sample, error) {
	return callWith401Retry(ctx, p, func(inner prom.Client) ([]prom.Sample, error) {
		return inner.Query(ctx, query, t)
	})
}

func (p *perShootClient) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]prom.TimeSeries, error) {
	return callWith401Retry(ctx, p, func(inner prom.Client) ([]prom.TimeSeries, error) {
		return inner.QueryRange(ctx, query, start, end, step)
	})
}

// callWith401Retry runs call against the currently cached inner client. On a
// 401 it invalidates the cache entry, re-resolves, and retries exactly once;
// if re-resolution fails, the original 401 is returned.
func callWith401Retry[T any](ctx context.Context, p *perShootClient, call func(prom.Client) (T, error)) (T, error) {
	var zero T
	inner, err := p.cache.resolved(ctx, p.clusterID)
	if err != nil {
		return zero, err
	}
	out, err := call(inner)
	if !isUnauthorized(err) {
		return out, err
	}
	inner, rerr := p.retryClient(ctx)
	if rerr != nil {
		return zero, err
	}
	return call(inner)
}

func (p *perShootClient) retryClient(ctx context.Context) (prom.Client, error) {
	p.cache.invalidate(p.clusterID)
	p.cache.logger.InfoContext(ctx, "per-shoot prometheus returned 401, re-resolving credentials", "cluster_id", p.clusterID)
	return p.cache.resolved(ctx, p.clusterID)
}

func isUnauthorized(err error) bool {
	var statusErr *prom.StatusError
	if !errors.As(err, &statusErr) {
		return false
	}
	return statusErr.StatusCode == http.StatusUnauthorized
}
