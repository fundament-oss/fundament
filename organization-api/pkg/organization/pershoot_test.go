package organization

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Deleted or idle clusters must not stay in memory for the process lifetime.
func TestPerShootCache_PrunesExpiredEntriesAndRetryMarks(t *testing.T) {
	current := time.Now()
	cache := newPerShootCache("test", slog.New(slog.DiscardHandler),
		func() time.Time { return current },
		func(context.Context, uuid.UUID) (string, error) { return "client", nil },
		time.Second,
	)
	gone, active := uuid.New(), uuid.New()

	_, err := cache.get(context.Background(), gone)
	require.NoError(t, err)
	require.True(t, cache.tryInvalidate(gone))
	_, err = cache.get(context.Background(), gone)
	require.NoError(t, err)

	current = current.Add(perShootTTL)
	_, err = cache.get(context.Background(), active)
	require.NoError(t, err)

	assert.NotContains(t, cache.entries, gone)
	assert.NotContains(t, cache.lastRetry, gone)
	assert.Contains(t, cache.entries, active)
}

func TestPerShootCache_InvalidateDropsRetryMark(t *testing.T) {
	cache := newPerShootCache("test", slog.New(slog.DiscardHandler), time.Now,
		func(context.Context, uuid.UUID) (string, error) { return "client", nil },
		time.Second,
	)
	id := uuid.New()

	require.True(t, cache.tryInvalidate(id))
	cache.invalidate(id)

	assert.NotContains(t, cache.lastRetry, id)
	assert.True(t, cache.tryInvalidate(id), "an explicit invalidation resets the retry window")
}
