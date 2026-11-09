package authz

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openfga/go-sdk/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// storesServer serves one ListStores page from the given raw store objects and
// records how many times it was asked.
func storesServer(t *testing.T, stores []map[string]any, calls *int) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls != nil {
			*calls++
		}

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"stores":             stores,
			"continuation_token": "",
		}))
	}))
	t.Cleanup(srv.Close)

	return srv
}

func newRef(t *testing.T, url string) *Store {
	t.Helper()

	store, err := NewStore(Config{APIURL: url, StoreName: "fundament"})
	require.NoError(t, err)

	return store
}

func TestResolvePicksTheOldestMatch(t *testing.T) {
	// Deliberately out of order, and with a decoy name.
	srv := storesServer(t, []map[string]any{
		{"id": "02newer", "name": "fundament", "created_at": "2026-01-02T00:00:00Z", "updated_at": "2026-01-02T00:00:00Z"},
		{"id": "01older", "name": "fundament", "created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z"},
		{"id": "03other", "name": "something-else", "created_at": "2025-01-01T00:00:00Z", "updated_at": "2025-01-01T00:00:00Z"},
	}, nil)

	ref := newRef(t, srv.URL)

	id, err := ref.refresh(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "01older", id, "every service must converge on the same store")
}

func TestResolveBreaksTiesOnID(t *testing.T) {
	// Stores created in the same instant must still order totally, or two
	// services can disagree about which one is current.
	srv := storesServer(t, []map[string]any{
		{"id": "bbb", "name": "fundament", "created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z"},
		{"id": "aaa", "name": "fundament", "created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z"},
	}, nil)

	ref := newRef(t, srv.URL)

	id, err := ref.refresh(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "aaa", id)
}

func TestResolveSkipsSoftDeletedStores(t *testing.T) {
	// A soft-deleted store still answers Check with allowed:true, so selecting
	// one would be silently wrong.
	srv := storesServer(t, []map[string]any{
		{
			"id": "01gone", "name": "fundament",
			"created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z",
			"deleted_at": "2026-01-03T00:00:00Z",
		},
		{"id": "02live", "name": "fundament", "created_at": "2026-01-02T00:00:00Z", "updated_at": "2026-01-02T00:00:00Z"},
	}, nil)

	ref := newRef(t, srv.URL)

	id, err := ref.refresh(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "02live", id)
}

func TestResolveReportsNoStore(t *testing.T) {
	srv := storesServer(t, nil, nil)
	ref := newRef(t, srv.URL)

	_, err := ref.refresh(context.Background())
	require.ErrorIs(t, err, ErrNoStore)
}

func TestIDCachesAfterFirstResolve(t *testing.T) {
	calls := 0
	srv := storesServer(t, []map[string]any{
		{"id": "01abc", "name": "fundament", "created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z"},
	}, &calls)

	ref := newRef(t, srv.URL)
	ctx := context.Background()

	for range 3 {
		id, err := ref.ID(ctx)
		require.NoError(t, err)
		assert.Equal(t, "01abc", id)
	}

	assert.Equal(t, 1, calls, "the store is resolved once, not on every check")
}

// Checks against an ignored store answer false with no error, so the warning is
// the only signal that a second store exists.
func TestResolveWarnsOnDuplicateStores(t *testing.T) {
	twoStores := []map[string]any{
		{"id": "01older", "name": "fundament", "created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z"},
		{"id": "02newer", "name": "fundament", "created_at": "2026-01-02T00:00:00Z", "updated_at": "2026-01-02T00:00:00Z"},
	}

	cases := []struct {
		name    string
		stores  []map[string]any
		wantLog bool
	}{
		{"one store is silent", twoStores[:1], false},
		{"two stores warn", twoStores, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer

			restore := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
			t.Cleanup(func() { slog.SetDefault(restore) })

			srv := storesServer(t, tc.stores, nil)
			fga, err := client.NewSdkClient(&client.ClientConfiguration{ApiUrl: srv.URL})
			require.NoError(t, err)

			id, err := ResolveStoreID(context.Background(), fga, "fundament")
			require.NoError(t, err)
			assert.Equal(t, "01older", id)

			logged := buf.String()
			if !tc.wantLog {
				assert.Empty(t, logged, "a single store must not warn")

				return
			}

			assert.Contains(t, logged, "several OpenFGA stores share one name")
			assert.Contains(t, logged, "using=01older", "the warning must name the store in use")
			assert.Contains(t, logged, "ignoring=02newer", "and the one being ignored")
			assert.Contains(t, logged, "found=2")
		})
	}
}

// removableStoreServer serves one store until gone is set, then answers as an
// OpenFGA whose store no longer exists.
func removableStoreServer(t *testing.T, gone *atomic.Bool) *httptest.Server {
	t.Helper()

	stored := map[string]any{"id": "01abc", "name": "fundament", "created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z"}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/stores" {
			stores := []map[string]any{stored}
			if gone.Load() {
				stores = nil
			}

			assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"stores": stores, "continuation_token": ""}))

			return
		}

		if gone.Load() {
			w.WriteHeader(http.StatusNotFound)
			assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"code": "store_id_not_found", "message": "store not found"}))

			return
		}

		assert.NoError(t, json.NewEncoder(w).Encode(stored))
	}))
	t.Cleanup(srv.Close)

	return srv
}

func TestReadyWhenStoreExists(t *testing.T) {
	var gone atomic.Bool

	ref := newRef(t, removableStoreServer(t, &gone).URL)

	require.NoError(t, ref.Ready(t.Context()))
}

// An outage must not take every replica out of its Service at once.
func TestReadyStaysReadyWhenOpenFGABecomesUnreachable(t *testing.T) {
	var gone atomic.Bool

	srv := removableStoreServer(t, &gone)
	ref := newRef(t, srv.URL)
	require.NoError(t, ref.Ready(t.Context()))

	srv.Close()

	require.NoError(t, ref.Ready(t.Context()))
}

// An error response hits every replica just as an unreachable OpenFGA does, and
// does not say the store is gone.
func TestReadyStaysReadyWhenOpenFGAAnswersWithAnError(t *testing.T) {
	var failing, gone atomic.Bool

	healthy := removableStoreServer(t, &gone).Config.Handler
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failing.Load() {
			w.WriteHeader(http.StatusBadRequest)

			return
		}

		healthy.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	ref := newRef(t, srv.URL)
	require.NoError(t, ref.Ready(t.Context()))

	failing.Store(true)

	require.NoError(t, ref.Ready(t.Context()))
}

// A pod that has never resolved the store cannot authorize anything, and helm
// --wait must not count it as deployed.
func TestReadyFailsWhenOpenFGAWasNeverReachable(t *testing.T) {
	var gone atomic.Bool

	srv := removableStoreServer(t, &gone)
	ref := newRef(t, srv.URL)
	srv.Close()

	require.Error(t, ref.Ready(t.Context()))
}

func TestReadyFailsUntilStoreIsProvisioned(t *testing.T) {
	var gone atomic.Bool
	gone.Store(true)

	srv := removableStoreServer(t, &gone)
	ref := newRef(t, srv.URL)
	require.ErrorIs(t, ref.Ready(t.Context()), ErrNoStore)

	gone.Store(false)

	require.NoError(t, ref.Ready(t.Context()))
}

// A lost store hits every replica alike, and only a deploy creates a new one.
func TestReadyStaysReadyWhenStoreDisappears(t *testing.T) {
	var gone atomic.Bool

	ref := newRef(t, removableStoreServer(t, &gone).URL)
	require.NoError(t, ref.Ready(t.Context()))

	gone.Store(true)

	require.NoError(t, ref.Ready(t.Context()))
}

// hangableServer serves one store until hanging is set, then answers nothing
// until the client gives up, as an OpenFGA that stopped responding.
func hangableServer(t *testing.T, hanging *atomic.Bool) *httptest.Server {
	t.Helper()

	var gone atomic.Bool

	healthy := removableStoreServer(t, &gone).Config.Handler
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hanging.Load() {
			<-r.Context().Done()

			return
		}

		healthy.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	return srv
}

// The probe handler has its own checks and the kubelet's timeout to fit in.
func TestReadyAnswersWithinBudgetWhenOpenFGAHangs(t *testing.T) {
	var hanging atomic.Bool

	ref := newRef(t, hangableServer(t, &hanging).URL)
	require.NoError(t, ref.Ready(t.Context()))

	hanging.Store(true)
	readyBudget = 200 * time.Millisecond
	t.Cleanup(func() { readyBudget = 2 * time.Second })

	start := time.Now()
	require.NoError(t, ref.Ready(t.Context()))
	assert.Less(t, time.Since(start), time.Second)
}
