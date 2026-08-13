package shoot

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"k8s.io/client-go/rest"
)

// clientCacheKey is the context key under which a batch's credential cache lives.
type clientCacheKey struct{}

// clientCache memoizes per-cluster shoot REST configs for the lifetime of one
// batch of ShootAccess calls.
type clientCache struct {
	mu      sync.Mutex
	configs map[uuid.UUID]*rest.Config
}

// WithClientCache scopes a shoot-credential cache to ctx: ShootAccess calls
// made with the returned context request one admin kubeconfig per cluster
// instead of a fresh one per verb. Each uncached build costs a Garden shoot
// lookup plus a CA-signed client certificate, so a handler that touches six
// resources on a shoot otherwise pays for six sets of credentials.
//
// Scope it to a single batch of operations. The credentials are deliberately
// short-lived, so a cache that outlives the batch would start handing out
// expired ones.
func WithClientCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, clientCacheKey{}, &clientCache{
		configs: make(map[uuid.UUID]*rest.Config),
	})
}

// clientCacheFrom returns the batch cache carried by ctx, or nil when the
// caller did not opt in.
func clientCacheFrom(ctx context.Context) *clientCache {
	cache, _ := ctx.Value(clientCacheKey{}).(*clientCache)
	return cache
}

// HasClientCache reports whether ctx carries a batch credential cache. Dropping
// the opt-in costs a set of credentials per verb without failing anything, so
// callers that rely on batching assert this in their tests.
func HasClientCache(ctx context.Context) bool {
	return clientCacheFrom(ctx) != nil
}

func (c *clientCache) get(clusterID uuid.UUID) (*rest.Config, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cfg, ok := c.configs[clusterID]
	return cfg, ok
}

func (c *clientCache) put(clusterID uuid.UUID, cfg *rest.Config) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.configs[clusterID] = cfg
}
