package token

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
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
