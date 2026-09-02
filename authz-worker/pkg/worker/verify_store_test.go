package worker

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/authz"
)

// provisioner stands in for the provision sidecar's status endpoint. An empty
// generation models a provisioner that is not answering yet.
func provisioner(t *testing.T, generation string) *authz.ProvisionedStore {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if generation == "" {
			w.WriteHeader(http.StatusServiceUnavailable)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"generation":"` + generation + `","store":"fundament","id":"s1"}`))
	}))
	t.Cleanup(srv.Close)

	return authz.NewProvisionedStore(srv.URL + "/status.json")
}

func gatedWorker(t *testing.T, release, served string) *Worker {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	return New(nil, nil, provisioner(t, served), logger, Config{Generation: release})
}

// Nothing published means there is no datastore to drain into.
func TestVerifyHoldsWhenNothingIsProvisioned(t *testing.T) {
	require.ErrorIs(t, gatedWorker(t, "release-1", "").verifyStore(t.Context()), errStoreUnavailable)
}

// A reset leaves the outgoing store in place until the wipe runs, so a store that
// exists is not enough: writing to it marks rows completed for tuples that are
// about to be destroyed.
func TestVerifyHoldsWhileOpenFGAServesAnotherGeneration(t *testing.T) {
	err := gatedWorker(t, "release-2", "release-1").verifyStore(t.Context())

	require.ErrorIs(t, err, errStoreUnavailable)
	require.Contains(t, err.Error(), "release-2")
	require.Contains(t, err.Error(), "release-1")
}

func TestVerifyPassesWhenTheGenerationMatches(t *testing.T) {
	require.NoError(t, gatedWorker(t, "release-2", "release-2").verifyStore(t.Context()))
}

// Without a configured generation the gate only requires something to be
// provisioned, which is the behaviour for a caller outside the chart.
func TestVerifyWithoutAGenerationChecksOnlyThatSomethingIsProvisioned(t *testing.T) {
	require.NoError(t, gatedWorker(t, "", "whatever").verifyStore(t.Context()))
}
