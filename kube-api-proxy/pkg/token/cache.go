// Package token manages ServiceAccount tokens for proxied cluster access: the
// per-user SAs and kube-api-proxy's own SA.
package token

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jellydator/ttlcache/v3"
	"golang.org/x/sync/singleflight"

	"github.com/fundament-oss/fundament/common/shootidentity"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/gardener"
)

// refreshRatio is the fraction of TTL at which to proactively refresh (80%).
const refreshRatio = 0.8

// cacheKey uniquely identifies a cached token: a ServiceAccount in
// fundament-system on one cluster.
type cacheKey struct {
	saName    string
	clusterID string
}

type cachedToken struct {
	token    string
	issuedAt time.Time
	ttl      time.Duration
}

// shouldRefresh returns true if the token has passed refreshRatio of its TTL.
func (ct *cachedToken) shouldRefresh() bool {
	refreshAt := ct.issuedAt.Add(time.Duration(float64(ct.ttl) * refreshRatio))
	return time.Now().After(refreshAt)
}

// Requester issues ServiceAccount tokens; *gardener.Client satisfies it.
type Requester interface {
	RequestNamedSAToken(ctx context.Context, clusterID, saName string) (*gardener.SAToken, error)
}

// Cache manages per-(ServiceAccount, cluster) tokens with proactive refresh.
type Cache struct {
	requester Requester
	logger    *slog.Logger
	tokens    *ttlcache.Cache[cacheKey, *cachedToken]
	group     singleflight.Group // deduplicates concurrent token requests
}

// NewCache creates a new token cache.
func NewCache(requester Requester, logger *slog.Logger) *Cache {
	c := &Cache{
		requester: requester,
		logger:    logger,
		tokens: ttlcache.New(
			// No default TTL — each entry gets its own TTL from the API server response.
			ttlcache.WithTTL[cacheKey, *cachedToken](ttlcache.NoTTL),
		),
	}
	// Start the automatic expired-entry cleanup goroutine.
	go c.tokens.Start()
	return c
}

// GetToken returns a valid token for the user's ServiceAccount on the cluster.
func (c *Cache) GetToken(ctx context.Context, userID uuid.UUID, clusterID string) (string, error) {
	return c.get(ctx, cacheKey{saName: "fundament-" + userID.String(), clusterID: clusterID})
}

// GetProxyToken returns a valid token for kube-api-proxy's own ServiceAccount
// on the cluster.
func (c *Cache) GetProxyToken(ctx context.Context, clusterID string) (string, error) {
	return c.get(ctx, cacheKey{saName: shootidentity.ProxyServiceAccount, clusterID: clusterID})
}

// get uses a cached token if available and not expired, triggering an async
// refresh if the token has passed 80% of its TTL.
func (c *Cache) get(ctx context.Context, key cacheKey) (string, error) {
	if item := c.tokens.Get(key); item != nil {
		ct := item.Value()
		if ct.shouldRefresh() {
			// Proactive async refresh — don't block the request.
			go c.refresh(context.WithoutCancel(ctx), key)
		}
		return ct.token, nil
	}

	// Cache miss or expired — synchronous fetch.
	return c.fetchAndCache(ctx, key)
}

func (c *Cache) fetchAndCache(ctx context.Context, key cacheKey) (string, error) {
	sfKey := fmt.Sprintf("%s:%s", key.saName, key.clusterID)

	v, err, _ := c.group.Do(sfKey, func() (any, error) {
		return c.requestToken(ctx, key)
	})
	if err != nil {
		return "", fmt.Errorf("fetch token for %s/%s: %w", key.saName, key.clusterID, err)
	}

	return v.(string), nil
}

func (c *Cache) refresh(ctx context.Context, key cacheKey) {
	sfKey := fmt.Sprintf("%s:%s", key.saName, key.clusterID)

	_, err, _ := c.group.Do(sfKey, func() (any, error) {
		return c.requestToken(ctx, key)
	})
	if err != nil {
		c.logger.WarnContext(ctx, "async token refresh failed",
			"service_account", key.saName, "cluster_id", key.clusterID, "error", err)
	}
}

func (c *Cache) requestToken(ctx context.Context, key cacheKey) (string, error) {
	saToken, err := c.requester.RequestNamedSAToken(ctx, key.clusterID, key.saName)
	if err != nil {
		return "", fmt.Errorf("request SA token: %w", err)
	}

	now := time.Now()
	ttl := saToken.ExpiresAt.Sub(now)
	ct := &cachedToken{
		token:    saToken.Token,
		issuedAt: now,
		ttl:      ttl,
	}
	// Store with actual TTL — ttlcache evicts automatically on expiry.
	c.tokens.Set(key, ct, ttl)

	c.logger.InfoContext(ctx, "SA token issued",
		"service_account", key.saName, "cluster_id", key.clusterID,
		"expires_at", saToken.ExpiresAt)

	return ct.token, nil
}
