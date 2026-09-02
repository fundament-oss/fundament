package authz

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// provisioner stands in for the provision sidecar's status endpoint. An empty
// store id models a provisioner that has not finished yet.
type provisioner struct {
	generation atomic.Value
	storeID    atomic.Value
	code       atomic.Int32
	reads      atomic.Int32
}

func newProvisioner(t *testing.T, storeID string) (*provisioner, *ProvisionedStore) {
	t.Helper()

	p := &provisioner{}
	p.publish("r1", storeID)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		p.reads.Add(1)

		if code := p.code.Load(); code != 0 {
			w.WriteHeader(int(code))

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"generation":"` + p.generation.Load().(string) +
			`","store":"fundament","id":"` + p.storeID.Load().(string) + `"}`))
	}))
	t.Cleanup(srv.Close)

	return p, NewProvisionedStore(srv.URL + "/status.json")
}

func (p *provisioner) publish(generation, storeID string) {
	p.generation.Store(generation)
	p.storeID.Store(storeID)
}

func TestDoUsesThePublishedStore(t *testing.T) {
	_, prov := newProvisioner(t, "store-1")

	var got string

	require.NoError(t, prov.Do(t.Context(), func(storeID string) error {
		got = storeID

		return nil
	}))
	assert.Equal(t, "store-1", got)
}

// A reset replaces the store while consumers hold the old id. Following it
// without a restart is the point of re-reading on failure.
func TestDoFollowsTheDatastoreWhenItMoves(t *testing.T) {
	srv, prov := newProvisioner(t, "store-1")

	var seen []string

	err := prov.Do(t.Context(), func(storeID string) error {
		seen = append(seen, storeID)
		if storeID == "store-1" {
			srv.publish("r2", "store-2")

			return errors.New("store is gone")
		}

		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"store-1", "store-2"}, seen, "must retry against the new store")
}

// Retrying a call that failed for its own reasons would double every error, so
// the original stands when the datastore has not moved.
func TestDoDoesNotRetryWhenNothingMoved(t *testing.T) {
	_, prov := newProvisioner(t, "store-1")

	calls := 0
	boom := errors.New("boom")

	err := prov.Do(t.Context(), func(string) error {
		calls++

		return boom
	})

	require.ErrorIs(t, err, boom)
	assert.Equal(t, 1, calls)
}

func TestDoCachesTheStatus(t *testing.T) {
	srv, prov := newProvisioner(t, "store-1")

	for range 5 {
		require.NoError(t, prov.Do(t.Context(), func(string) error { return nil }))
	}

	assert.Equal(t, int32(1), srv.reads.Load())
}

func TestDoFailsClosedWhenTheProvisionerIsUnreachable(t *testing.T) {
	srv, prov := newProvisioner(t, "store-1")
	srv.code.Store(http.StatusServiceUnavailable)

	called := false
	err := prov.Do(t.Context(), func(string) error {
		called = true

		return nil
	})

	require.ErrorIs(t, err, ErrNotProvisioned)
	assert.False(t, called, "nothing may run without a store to run it against")
}

// A provisioner that answers before it has created the store is not an answer.
func TestDoFailsClosedOnAnEmptyStoreID(t *testing.T) {
	_, prov := newProvisioner(t, "")

	err := prov.Do(t.Context(), func(string) error { return nil })

	require.ErrorIs(t, err, ErrNotProvisioned)
}

// The generation decides whether the datastore in front of a caller is its own,
// so it must never come from cache.
func TestGenerationIsReadFresh(t *testing.T) {
	srv, prov := newProvisioner(t, "store-1")

	first, err := prov.Generation(t.Context())
	require.NoError(t, err)

	srv.publish("r2", "store-2")

	second, err := prov.Generation(t.Context())
	require.NoError(t, err)

	assert.Equal(t, "r1", first)
	assert.Equal(t, "r2", second)
}
