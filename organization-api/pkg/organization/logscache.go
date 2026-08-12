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
	"github.com/fundament-oss/fundament/organization-api/pkg/logs"
)

// perShootLogs resolves and caches one Vali client per cluster. Vali has no
// ingress of its own (only its push endpoint is exposed), so the client
// targets the shoot Plutono's datasource proxy: the "plutono-url" annotation
// on the Gardener monitoring secret plus a probed numeric datasource id
// (logs.DiscoverValiProxyBase), authenticated with the secret's basic-auth
// credentials. Verified live 2026-08-04 (ADR-0027).
type perShootLogs struct {
	gardener gardener.Client
	logger   *slog.Logger
	now      func() time.Time

	// newClient and discover are seams for tests; production wiring uses
	// logs.NewLokiClientWithAuth and logs.DiscoverValiProxyBase.
	newClient func(base, username, password string) logs.Client
	discover  func(ctx context.Context, plutonoURL, username, password string) (string, error)

	cache *perShootCache[logs.Client]
}

func newPerShootLogs(g gardener.Client, logger *slog.Logger, opts ...logs.Option) *perShootLogs {
	c := &perShootLogs{
		gardener: g,
		logger:   logger,
		now:      time.Now,
		newClient: func(base, username, password string) logs.Client {
			return logs.NewLokiClientWithAuth(base, username, password, opts...)
		},
		discover: func(ctx context.Context, plutonoURL, username, password string) (string, error) {
			return logs.DiscoverValiProxyBase(ctx, plutonoURL, username, password, opts...)
		},
	}
	c.cache = newPerShootCache(func() time.Time { return c.now() }, c.resolveClient)
	return c
}

func (c *perShootLogs) resolveClient(ctx context.Context, clusterID uuid.UUID) (logs.Client, error) {
	info, err := c.gardener.Monitoring(ctx, clusterID)
	if err != nil {
		return nil, fmt.Errorf("look up shoot monitoring: %w", err)
	}
	if info.URL == "" {
		return nil, fmt.Errorf("monitoring secret has no plutono-url annotation: %w", gardener.ErrNotFound)
	}
	base, err := c.discover(ctx, info.URL, info.Username, info.Password)
	if err != nil {
		return nil, fmt.Errorf("discover vali datasource: %w", err)
	}
	return c.newClient(base, info.Username, info.Password), nil
}

// clientFor returns a client for the cluster's per-shoot Vali. It returns
// gardener.ErrNotFound (possibly wrapped) while the shoot or its monitoring
// stack is not available yet, and logs.ErrValiNotFound when Plutono answers
// but no datasource proxies the Vali API; callers degrade to "no logs" on
// both. The returned client re-resolves once on 401 (credential rotation) and
// on 404/5xx (datasource-id drift), so both heal without a restart.
func (c *perShootLogs) clientFor(ctx context.Context, clusterID uuid.UUID) (logs.Client, error) {
	_, err := c.cache.get(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	return &perShootLogsClient{cache: c, clusterID: clusterID}, nil
}

// perShootLogsClient is the stable handle handed to the RPC layer. Each call
// fetches the currently cached inner client; when the backend answers with a
// status that suggests stale resolution it invalidates, re-resolves (secret +
// datasource id), and retries exactly once.
type perShootLogsClient struct {
	cache     *perShootLogs
	clusterID uuid.UUID
}

func (p *perShootLogsClient) Backend() logs.Backend { return logs.BackendLoki }

func (p *perShootLogsClient) Query(ctx context.Context, params *logs.QueryParams) ([]logs.Entry, error) {
	return callWithReResolve(ctx, p, func(inner logs.Client) ([]logs.Entry, error) {
		return inner.Query(ctx, params)
	})
}

// Tail cannot lean on callWithReResolve the way the request/response calls do:
// Tail returns before the stream fails, so a stale resolution shows up as a
// terminal TailEvent minutes later. The retry therefore lives in the forwarding
// goroutine — on the first such failure the entry is invalidated, the client
// re-resolved, and a fresh inner tail spliced onto the same output channel, so
// credential rotation heals mid-stream instead of ending the tail.
func (p *perShootLogsClient) Tail(ctx context.Context, params *logs.QueryParams) (<-chan logs.TailEvent, error) {
	inner, err := callWithReResolve(ctx, p, func(inner logs.Client) (<-chan logs.TailEvent, error) {
		return inner.Tail(ctx, params)
	})
	if err != nil {
		return nil, err
	}

	out := make(chan logs.TailEvent)
	go func() {
		defer close(out)
		current := inner
		retried := false
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-current:
				if !ok {
					return
				}
				if ev.Err != nil && !retried && isStaleResolution(ev.Err) {
					retried = true
					p.cache.cache.invalidate(p.clusterID)
					p.cache.logger.InfoContext(ctx, "per-shoot vali tail hit a stale-resolution status, re-resolving",
						"cluster_id", p.clusterID, "error", ev.Err)
					next, rerr := p.restartTail(ctx, params)
					if rerr != nil {
						p.forward(ctx, out, &ev)
						return
					}
					// The new tail starts at "now", so the few seconds spent
					// re-resolving are not backfilled.
					current = next
					continue
				}
				p.forward(ctx, out, &ev)
				if ev.Err != nil {
					return
				}
			}
		}
	}()
	return out, nil
}

func (p *perShootLogsClient) restartTail(ctx context.Context, params *logs.QueryParams) (<-chan logs.TailEvent, error) {
	inner, err := p.cache.cache.get(ctx, p.clusterID)
	if err != nil {
		return nil, err
	}
	ch, err := inner.Tail(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("restart tail: %w", err)
	}
	return ch, nil
}

func (*perShootLogsClient) forward(ctx context.Context, out chan<- logs.TailEvent, ev *logs.TailEvent) {
	select {
	case out <- *ev:
	case <-ctx.Done():
	}
}

func (p *perShootLogsClient) Labels(ctx context.Context, clusterID, namespace string, start, end time.Time) (logs.Labels, error) {
	return callWithReResolve(ctx, p, func(inner logs.Client) (logs.Labels, error) {
		return inner.Labels(ctx, clusterID, namespace, start, end)
	})
}

// callWithReResolve runs call against the currently cached inner client. On a
// 401 (rotated credentials), 404, or 5xx (datasource id drifted, e.g. after a
// Plutono re-provision) it invalidates the cache entry, re-resolves, and
// retries exactly once; if re-resolution fails, the original error is
// returned. A 400 (bad query) is never retried.
func callWithReResolve[T any](ctx context.Context, p *perShootLogsClient, call func(logs.Client) (T, error)) (T, error) {
	var zero T
	inner, err := p.cache.cache.get(ctx, p.clusterID)
	if err != nil {
		return zero, err
	}
	out, err := call(inner)
	if !isStaleResolution(err) {
		return out, err
	}
	p.cache.cache.invalidate(p.clusterID)
	p.cache.logger.InfoContext(ctx, "per-shoot vali answered with a stale-resolution status, re-resolving",
		"cluster_id", p.clusterID, "error", err)
	inner, rerr := p.cache.cache.get(ctx, p.clusterID)
	if rerr != nil {
		return zero, err
	}
	return call(inner)
}

func isStaleResolution(err error) bool {
	var statusErr *logs.StatusError
	if !errors.As(err, &statusErr) {
		return false
	}
	return statusErr.StatusCode == http.StatusUnauthorized ||
		statusErr.StatusCode == http.StatusNotFound ||
		statusErr.StatusCode >= http.StatusInternalServerError
}
