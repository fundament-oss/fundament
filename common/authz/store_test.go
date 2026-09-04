package authz

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

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

func newRef(t *testing.T, url string) (*storeRef, *client.OpenFgaClient) {
	t.Helper()

	fga, err := client.NewSdkClient(&client.ClientConfiguration{ApiUrl: url})
	require.NoError(t, err)

	return &storeRef{name: "fundament"}, fga
}

func TestResolvePicksTheOldestMatch(t *testing.T) {
	// Deliberately out of order, and with a decoy name.
	srv := storesServer(t, []map[string]any{
		{"id": "02newer", "name": "fundament", "created_at": "2026-01-02T00:00:00Z", "updated_at": "2026-01-02T00:00:00Z"},
		{"id": "01older", "name": "fundament", "created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z"},
		{"id": "03other", "name": "something-else", "created_at": "2025-01-01T00:00:00Z", "updated_at": "2025-01-01T00:00:00Z"},
	}, nil)

	ref, fga := newRef(t, srv.URL)

	id, err := ref.resolve(context.Background(), fga)
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

	ref, fga := newRef(t, srv.URL)

	id, err := ref.resolve(context.Background(), fga)
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

	ref, fga := newRef(t, srv.URL)

	id, err := ref.resolve(context.Background(), fga)
	require.NoError(t, err)
	assert.Equal(t, "02live", id)
}

func TestResolveReportsNoStore(t *testing.T) {
	srv := storesServer(t, nil, nil)
	ref, fga := newRef(t, srv.URL)

	_, err := ref.resolve(context.Background(), fga)
	require.ErrorIs(t, err, ErrNoStore)
}

func TestGetCachesAfterFirstResolve(t *testing.T) {
	calls := 0
	srv := storesServer(t, []map[string]any{
		{"id": "01abc", "name": "fundament", "created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z"},
	}, &calls)

	ref, fga := newRef(t, srv.URL)
	ctx := context.Background()

	for range 3 {
		id, err := ref.get(ctx, fga)
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
