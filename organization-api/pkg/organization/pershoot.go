package organization

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
)

const (
	// perShootTTL bounds how long a resolved per-shoot client is reused before
	// the monitoring secret is re-read. Entries expire at 70% of the TTL so
	// refreshes happen before staleness bites (same convention as
	// kube-api-proxy's admin-kubeconfig cache).
	perShootTTL = 10 * time.Minute

	// defaultResolveTimeout bounds one resolution. Resolution runs detached from
	// the winning caller's context (all concurrent callers join its flight), so
	// it needs its own deadline. Owners whose resolution does more work pass
	// their own: this default was sized for a single secret read, and silently
	// cut a resolver that probes several endpoints in sequence.
	defaultResolveTimeout = 15 * time.Second

	// negativeTTL bounds how long a failed resolution is remembered. Without it
	// every request against a cluster with no reachable backend re-ran the whole
	// resolution — a secret read plus, for logs, a sequential probe — because
	// singleflight only dedupes callers that overlap in time. A logs page fires
	// several requests and repeats them on every filter change, so one broken
	// cluster meant a stall per request and a warning per request, for a result
	// already known.
	negativeTTL = 30 * time.Second
)

// perShootCache caches one resolved value per cluster with TTL expiry,
// singleflight-deduped resolution, and explicit invalidation. It is the
// mechanics shared by the per-shoot Prometheus (metrics) and Vali (logs)
// resolvers; what "resolving" means is injected by the owner.
type perShootCache[T any] struct {
	// now and resolve are indirections so owners can expose their own test
	// seams; both must be set before use.
	now     func() time.Time
	resolve func(ctx context.Context, clusterID uuid.UUID) (T, error)
	// resolveTimeout bounds one resolution attempt.
	resolveTimeout time.Duration

	mu      sync.Mutex
	entries map[uuid.UUID]perShootCacheEntry[T]
	sf      singleflight.Group
}

type perShootCacheEntry[T any] struct {
	value T
	// err, when non-nil, is a remembered failure: the entry is negative and
	// callers get this error back without re-resolving until it expires.
	err     error
	expires time.Time
}

func newPerShootCache[T any](
	now func() time.Time,
	resolve func(ctx context.Context, clusterID uuid.UUID) (T, error),
	resolveTimeout time.Duration,
) *perShootCache[T] {
	if resolveTimeout <= 0 {
		resolveTimeout = defaultResolveTimeout
	}
	return &perShootCache[T]{
		now:            now,
		resolve:        resolve,
		resolveTimeout: resolveTimeout,
		entries:        make(map[uuid.UUID]perShootCacheEntry[T]),
	}
}

// get returns the cached value for clusterID, resolving it when absent or
// expired. Concurrent resolutions for the same cluster are deduped.
func (c *perShootCache[T]) get(ctx context.Context, clusterID uuid.UUID) (T, error) {
	var zero T

	c.mu.Lock()
	entry, ok := c.entries[clusterID]
	c.mu.Unlock()
	if ok && c.now().Before(entry.expires) {
		if entry.err != nil {
			return zero, entry.err
		}
		return entry.value, nil
	}

	v, err, _ := c.sf.Do(clusterID.String(), func() (any, error) {
		// Detach from the winning caller's cancellation: every concurrent
		// caller for this cluster joins this flight, and one caller aborting
		// (page refresh) must not fail the others' resolution.
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.resolveTimeout)
		defer cancel()
		value, err := c.resolve(rctx, clusterID)
		if err != nil {
			c.mu.Lock()
			c.entries[clusterID] = perShootCacheEntry[T]{
				err:     err,
				expires: c.now().Add(negativeTTL),
			}
			c.mu.Unlock()
			return nil, err
		}
		c.mu.Lock()
		c.entries[clusterID] = perShootCacheEntry[T]{
			value:   value,
			expires: c.now().Add(perShootTTL * 7 / 10),
		}
		c.mu.Unlock()
		return value, nil
	})
	if err != nil {
		return zero, err //nolint:wrapcheck // the error comes from the owner's resolve closure, which already adds context
	}
	return v.(T), nil
}

func (c *perShootCache[T]) invalidate(clusterID uuid.UUID) {
	c.mu.Lock()
	delete(c.entries, clusterID)
	c.mu.Unlock()
}
