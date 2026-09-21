package psqldb

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recorder is a SessionSetting whose hooks record calls and fail on demand.
type recorder struct {
	name     string
	calls    *[]string
	setErr   error
	resetErr error
	notInCtx bool
}

func (r recorder) setting() SessionSetting {
	return SessionSetting{
		Name: r.name,
		Set: func(context.Context, *pgx.Conn) (bool, error) {
			*r.calls = append(*r.calls, "set "+r.name)
			return !r.notInCtx, r.setErr
		},
		Reset: func(context.Context, *pgx.Conn) error {
			*r.calls = append(*r.calls, "reset "+r.name)
			return r.resetErr
		},
	}
}

func poolConfig(t *testing.T, settings ...SessionSetting) *pgxpool.Config {
	t.Helper()
	cfg, err := pgxpool.ParseConfig("postgres://test@localhost/test")
	require.NoError(t, err)
	WithSessionSettings(slog.New(slog.NewTextHandler(io.Discard, nil)), settings...)(context.Background(), cfg)
	return cfg
}

func TestWithSessionSettings_AppliesAndResetsInOrder(t *testing.T) {
	var calls []string
	cfg := poolConfig(t,
		recorder{name: "organization", calls: &calls}.setting(),
		recorder{name: "user", calls: &calls, notInCtx: true}.setting(),
	)

	ok, err := cfg.PrepareConn(context.Background(), nil)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.True(t, cfg.AfterRelease(nil))
	assert.Equal(t, []string{"set organization", "set user", "reset organization", "reset user"}, calls)
}

func TestWithSessionSettings_FailedSetRefusesTheConnection(t *testing.T) {
	var calls []string
	cfg := poolConfig(t, recorder{name: "organization", calls: &calls, setErr: errors.New("boom")}.setting())

	ok, err := cfg.PrepareConn(context.Background(), nil)
	require.ErrorContains(t, err, "failed to set organization context")
	assert.False(t, ok)
}

// A failed reset must destroy the connection: handing it back would leak one
// caller's identity into the next request.
func TestWithSessionSettings_FailedResetDestroysTheConnection(t *testing.T) {
	var calls []string
	cfg := poolConfig(t,
		recorder{name: "organization", calls: &calls, resetErr: errors.New("boom")}.setting(),
		recorder{name: "user", calls: &calls}.setting(),
	)

	assert.False(t, cfg.AfterRelease(nil))
	assert.Equal(t, []string{"reset organization"}, calls, "stops at the first failure")
}
