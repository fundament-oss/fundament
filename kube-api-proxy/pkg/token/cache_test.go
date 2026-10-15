package token

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jellydator/ttlcache/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/gardener"
)

type fakeRequester struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeRequester) RequestNamedSAToken(_ context.Context, clusterID, saName string) (*gardener.SAToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, clusterID+"/"+saName)
	return &gardener.SAToken{Token: "tok-" + saName, ExpiresAt: time.Now().Add(15 * time.Minute)}, nil
}

func TestCache_UserAndProxyTokensAreSeparateAndCached(t *testing.T) {
	t.Parallel()
	req := &fakeRequester{}
	c := NewCache(req, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()
	userID := uuid.New()

	userTok, err := c.GetToken(ctx, userID, "c1")
	require.NoError(t, err)
	proxyTok, err := c.GetProxyToken(ctx, "c1")
	require.NoError(t, err)
	_, err = c.GetProxyToken(ctx, "c1")
	require.NoError(t, err)

	assert.Equal(t, "tok-fundament-"+userID.String(), userTok)
	assert.Equal(t, "tok-fundament-kube-api-proxy", proxyTok)
	assert.Equal(t, []string{"c1/fundament-" + userID.String(), "c1/fundament-kube-api-proxy"}, req.calls,
		"the second proxy token came from the cache")
}

type failingRequester struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *failingRequester) RequestNamedSAToken(context.Context, string, string) (*gardener.SAToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &gardener.SAToken{Token: "tok", ExpiresAt: time.Now().Add(15 * time.Minute)}, nil
}

// TestCache_FailureIsCachedBriefly pins that a cluster still being created
// costs one Gardener round trip per failureTTL, not one per kubectl call.
func TestCache_FailureIsCachedBriefly(t *testing.T) {
	t.Parallel()
	req := &failingRequester{err: gardener.ErrShootNotFound}
	c := NewCache(req, slog.New(slog.NewTextHandler(io.Discard, nil)))
	userID := uuid.New()

	_, err := c.GetToken(context.Background(), userID, "c1")
	require.ErrorIs(t, err, gardener.ErrShootNotFound)
	_, err = c.GetToken(context.Background(), userID, "c1")
	require.ErrorIs(t, err, gardener.ErrShootNotFound, "the cached failure keeps its cause")
	assert.Equal(t, 1, req.calls)

	_, err = c.GetToken(context.Background(), userID, "c2")
	require.Error(t, err)
	assert.Equal(t, 2, req.calls, "failures are cached per cluster")
}

func TestCache_FailureExpires(t *testing.T) {
	t.Parallel()
	req := &failingRequester{err: gardener.ErrShootNotFound}
	c := NewCache(req, slog.New(slog.NewTextHandler(io.Discard, nil)))
	c.failures = newFailureCache(50 * time.Millisecond)
	userID := uuid.New()

	_, err := c.GetToken(context.Background(), userID, "c1")
	require.Error(t, err)

	req.mu.Lock()
	req.err = nil
	req.mu.Unlock()
	require.Eventually(t, func() bool {
		tok, err := c.GetToken(context.Background(), userID, "c1")
		return err == nil && tok == "tok"
	}, time.Second, 20*time.Millisecond, "a failure is only answered from the cache for failureTTL")
	assert.Equal(t, 2, req.calls)
}

// TestCache_FailedRefreshIsThrottled pins that a token past its refresh point
// whose refresh failed is not refreshed again on every request.
func TestCache_FailedRefreshIsThrottled(t *testing.T) {
	t.Parallel()
	req := &failingRequester{}
	c := NewCache(req, slog.New(slog.NewTextHandler(io.Discard, nil)))
	key := cacheKey{saName: "fundament-kube-api-proxy", clusterID: "c1"}
	c.tokens.Set(key, &cachedToken{token: "old", issuedAt: time.Now().Add(-time.Hour), ttl: 2 * time.Hour}, time.Hour)
	c.failures.Set(key, gardener.ErrSyncPending, ttlcache.DefaultTTL)

	for range 5 {
		tok, err := c.GetProxyToken(context.Background(), "c1")
		require.NoError(t, err)
		assert.Equal(t, "old", tok)
	}
	time.Sleep(50 * time.Millisecond)
	req.mu.Lock()
	defer req.mu.Unlock()
	assert.Zero(t, req.calls, "no refresh while the last one failed recently")
}

func TestCache_CancelledRequestIsNotCached(t *testing.T) {
	t.Parallel()
	req := &failingRequester{err: context.Canceled}
	c := NewCache(req, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.GetProxyToken(ctx, "c1")
	require.Error(t, err)

	req.mu.Lock()
	req.err = nil
	req.mu.Unlock()
	tok, err := c.GetProxyToken(context.Background(), "c1")
	require.NoError(t, err)
	assert.Equal(t, "tok", tok)
}

// TestCache_ForgetDropsOnlyTheRejectedToken pins that a token a cluster
// rejects, as after the cluster was created again, is fetched anew, while a
// token fetched since is kept.
func TestCache_ForgetDropsOnlyTheRejectedToken(t *testing.T) {
	t.Parallel()
	req := &fakeRequester{}
	c := NewCache(req, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()
	userID := uuid.New()

	userTok, err := c.GetToken(ctx, userID, "c1")
	require.NoError(t, err)
	proxyTok, err := c.GetProxyToken(ctx, "c1")
	require.NoError(t, err)

	c.ForgetToken(userID, "c1", "an older token")
	c.ForgetProxyToken("c1", proxyTok)
	_, err = c.GetToken(ctx, userID, "c1")
	require.NoError(t, err)
	_, err = c.GetProxyToken(ctx, "c1")
	require.NoError(t, err)
	assert.Len(t, req.calls, 3, "only the proxy token was fetched again")

	c.ForgetToken(userID, "c1", userTok)
	_, err = c.GetToken(ctx, userID, "c1")
	require.NoError(t, err)
	assert.Len(t, req.calls, 4, "the rejected user token was fetched again")
}
