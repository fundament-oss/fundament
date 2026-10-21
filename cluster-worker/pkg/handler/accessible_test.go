package handler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
)

func TestReconcileAccessible(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	unreachable := &net.DNSError{Err: "no such host", Name: "api.example", IsNotFound: true}
	newCluster, readyCluster, brokenCluster := uuid.New(), uuid.New(), uuid.New()
	clusters := []db.ClusterListAccessibleRow{
		{ID: newCluster, Ready: false},
		{ID: readyCluster, Ready: true},
		{ID: brokenCluster, Ready: false},
	}
	errs := map[uuid.UUID]error{
		newCluster:    unreachable,
		readyCluster:  unreachable,
		brokenCluster: errors.New("forbidden"),
	}

	var visited []uuid.UUID
	err := ReconcileAccessible(context.Background(), logger, clusters, func(_ context.Context, id uuid.UUID) error {
		visited = append(visited, id)
		return errs[id]
	})

	assert.Equal(t, []uuid.UUID{newCluster, readyCluster, brokenCluster}, visited)
	require.Error(t, err)
	assert.ErrorIs(t, err, unreachable, "a ready cluster that does not answer is a failure")
	assert.ErrorContains(t, err, "forbidden", "other errors are failures")
	assert.Len(t, unwrapJoined(err), 2, "only the not-ready unreachable cluster is skipped")
}

func TestReconcileAccessibleStopsOnShutdown(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := ReconcileAccessible(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)),
		[]db.ClusterListAccessibleRow{{ID: uuid.New(), Ready: true}},
		func(context.Context, uuid.UUID) error { called = true; return nil })
	require.NoError(t, err)
	assert.False(t, called)
}

func unwrapJoined(err error) []error {
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return []error{err}
	}
	return joined.Unwrap()
}
