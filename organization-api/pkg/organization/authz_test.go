package organization

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/authz"
)

// unreachableAuthzServer is a Server whose OpenFGA cannot be reached, as in an
// outage: the address it has was closed before asking, so resolving the store
// already fails.
func unreachableAuthzServer(t *testing.T) *Server {
	t.Helper()

	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()

	return authzServerAt(t, gone.URL)
}

func authzServerAt(t *testing.T, url string) *Server {
	t.Helper()

	store, err := authz.NewStore(authz.Config{APIURL: url, StoreName: "fundament"})
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

// A check OpenFGA turns down, here for a relation its model lacks, is turned
// down again on the next try: that is not an outage to wait out.
func TestCheckPermission_AuthzRefusesTheRequest(t *testing.T) {
	fga := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/stores" {
			_, _ = w.Write([]byte(`{"stores":[{"id":"01ARZ3NDEKTSV4RRFFQ69G5FAV","name":"fundament"}]}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"validation_error","message":"relation not found"}`))
	}))
	t.Cleanup(fga.Close)

	s := authzServerAt(t, fga.URL)
	ctx := WithUserID(t.Context(), uuid.New())

	err := s.checkPermission(ctx, authz.CanView(), authz.Organization(uuid.New()))

	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.NotContains(t, err.Error(), "127.0.0.1")
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
