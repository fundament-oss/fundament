package organization

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/authz"
)

// unreachableAuthzServer is a Server whose OpenFGA cannot be reached, as in an
// outage: nothing listens on port 1, so resolving the store already fails.
func unreachableAuthzServer(t *testing.T) *Server {
	t.Helper()

	store, err := authz.NewStore(authz.Config{APIURL: "http://127.0.0.1:1", StoreName: "fundament"})
	require.NoError(t, err)

	return &Server{
		authz:  authz.NewClient(store),
		logger: slog.New(slog.DiscardHandler),
	}
}

func TestCheckPermission_AuthzUnreachable(t *testing.T) {
	s := unreachableAuthzServer(t)
	ctx := WithUserID(t.Context(), uuid.New())

	err := s.checkPermission(ctx, authz.CanView(), authz.Organization(uuid.New()))

	require.Error(t, err)
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
	assert.NotContains(t, err.Error(), "127.0.0.1")
	assert.NotContains(t, err.Error(), "http")
}

func TestCheckPermission_CallerGoneIsNotAnOutage(t *testing.T) {
	s := unreachableAuthzServer(t)

	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(WithUserID(t.Context(), uuid.New()))
		cancel()

		err := s.checkPermission(ctx, authz.CanView(), authz.Organization(uuid.New()))

		assert.Equal(t, connect.CodeCanceled, connect.CodeOf(err))
	})

	t.Run("deadline exceeded", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(WithUserID(t.Context(), uuid.New()), time.Now().Add(-time.Second))
		defer cancel()

		err := s.checkPermission(ctx, authz.CanView(), authz.Organization(uuid.New()))

		assert.Equal(t, connect.CodeDeadlineExceeded, connect.CodeOf(err))
	})
}
