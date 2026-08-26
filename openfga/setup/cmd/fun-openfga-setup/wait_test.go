package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// statusServer answers with whatever generation it currently holds, so a test can
// model the previous release's provisioner still being up.
type statusServer struct {
	generation atomic.Value
	calls      atomic.Int32
}

func (s *statusServer) start(t *testing.T) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		s.calls.Add(1)

		gen, _ := s.generation.Load().(string)
		if gen == "" {
			w.WriteHeader(http.StatusServiceUnavailable)

			return
		}

		_ = json.NewEncoder(w).Encode(Status{Generation: gen, Store: "fundament", StoreID: "01ABC"})
	}))
	t.Cleanup(srv.Close)

	return srv.URL + statusPath
}

func TestWaitReturnsWhenTheGenerationMatches(t *testing.T) {
	srv := &statusServer{}
	srv.generation.Store("7")

	require.NoError(t, Wait(t.Context(), WaitConfig{StatusURL: srv.start(t), Generation: "7", Timeout: 5 * time.Second}))
}

func TestWaitIgnoresAnotherReleasesGeneration(t *testing.T) {
	srv := &statusServer{}
	srv.generation.Store("6")

	err := Wait(t.Context(), WaitConfig{StatusURL: srv.start(t), Generation: "7", Timeout: 3 * time.Second})

	require.Error(t, err, "a previous release's status must never satisfy the wait")
	assert.Contains(t, err.Error(), `generation "7"`)
}

func TestWaitBlocksUntilTheGenerationAppears(t *testing.T) {
	srv := &statusServer{}
	url := srv.start(t)

	go func() {
		time.Sleep(3 * time.Second)
		srv.generation.Store("7")
	}()

	require.NoError(t, Wait(t.Context(), WaitConfig{StatusURL: url, Generation: "7", Timeout: 20 * time.Second}))
	assert.Greater(t, srv.calls.Load(), int32(1), "it should have polled while waiting")
}

func TestWaitTimesOutWhenNothingAnswers(t *testing.T) {
	err := Wait(t.Context(), WaitConfig{StatusURL: "http://127.0.0.1:1/status.json", Generation: "7", Timeout: 2 * time.Second})

	require.Error(t, err)
}

func TestWaitRequiresItsArguments(t *testing.T) {
	require.Error(t, Wait(t.Context(), WaitConfig{Generation: "7", Timeout: time.Second}))
	require.Error(t, Wait(t.Context(), WaitConfig{StatusURL: "http://x/status.json", Timeout: time.Second}))
}
