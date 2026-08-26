package worker

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/authz"
)

// storeServer serves just enough of the store API for verifyStore.
type storeServer struct {
	mu     sync.Mutex
	stores []openfga.Store
}

func (s *storeServer) set(stores ...openfga.Store) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stores = stores
}

func (s *storeServer) resolver(t *testing.T) *authz.StoreResolver {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openfga.ListStoresResponse{Stores: s.stores})
	}))
	t.Cleanup(srv.Close)

	fga, err := client.NewSdkClient(&client.ClientConfiguration{ApiUrl: srv.URL})
	require.NoError(t, err)

	return authz.NewStoreResolver(fga, "fundament")
}

func newVerifyWorker(t *testing.T, srv *storeServer) *Worker {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	return New(nil, nil, srv.resolver(t), nil, logger, Config{})
}

// statusServer stands in for the provision sidecar's status endpoint.
func statusServer(t *testing.T, generation string) *authz.StatusClient {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"generation":"` + generation + `","store":"fundament","id":"x"}`))
	}))
	t.Cleanup(srv.Close)

	return authz.NewStatusClient(srv.URL + "/status.json")
}

// newGatedWorker builds a worker that gates drains on a release generation.
func newGatedWorker(t *testing.T, srv *storeServer, release, served string) *Worker {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	return New(nil, nil, srv.resolver(t), statusServer(t, served), logger,
		Config{Generation: release})
}

func store(id string, created time.Time) openfga.Store {
	return openfga.Store{Id: id, Name: "fundament", CreatedAt: created, UpdatedAt: created}
}

func TestVerifyStoreHoldsWhenNoStoreExists(t *testing.T) {
	err := newVerifyWorker(t, &storeServer{}).verifyStore(t.Context())

	require.ErrorIs(t, err, errStoreUnavailable)
	require.ErrorIs(t, err, authz.ErrStoreNotFound)
}

// A reset replaces the store; the worker must hold while none exists, then
// follow the replacement without a restart.
func TestVerifyStoreRecoversAgainstAReplacementStore(t *testing.T) {
	now := time.Now()
	srv := &storeServer{}
	srv.set(store("01M0Y00000000000000000000A", now))
	w := newVerifyWorker(t, srv)

	require.NoError(t, w.verifyStore(t.Context()))

	srv.set()
	require.ErrorIs(t, w.verifyStore(t.Context()), errStoreUnavailable)

	srv.set(store("01M0Y00000000000000000000B", now.Add(time.Hour)))
	require.NoError(t, w.verifyStore(t.Context()))

	id, err := w.store.ID(t.Context())
	require.NoError(t, err)
	require.Equal(t, "01M0Y00000000000000000000B", id, "the next drain must target the replacement")
}

// A store that exists is not enough: during a reset the outgoing store is still
// there, and writing to it marks rows completed for tuples about to be destroyed.
func TestVerifyHoldsWhileOpenFGAServesAnotherGeneration(t *testing.T) {
	srv := &storeServer{}
	srv.set(store("01JMZ0000000000000000000AA", time.Now()))

	err := newGatedWorker(t, srv, "release-2", "release-1").verifyStore(t.Context())

	require.ErrorIs(t, err, errStoreUnavailable)
	require.Contains(t, err.Error(), "release-2")
	require.Contains(t, err.Error(), "release-1")
}

func TestVerifyPassesWhenTheGenerationMatches(t *testing.T) {
	srv := &storeServer{}
	srv.set(store("01JMZ0000000000000000000AA", time.Now()))

	require.NoError(t, newGatedWorker(t, srv, "release-2", "release-2").verifyStore(t.Context()))
}

// Without a configured generation the gate is inert, so a caller outside the
// chart keeps the old behaviour of waiting only for the store to exist.
func TestVerifyWithoutAGenerationChecksOnlyExistence(t *testing.T) {
	srv := &storeServer{}
	srv.set(store("01JMZ0000000000000000000AA", time.Now()))

	require.NoError(t, newGatedWorker(t, srv, "", "whatever").verifyStore(t.Context()))
}
