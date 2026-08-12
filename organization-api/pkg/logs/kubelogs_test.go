package logs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKubeClient_Query(t *testing.T) {
	var gotPath, gotAuth string
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(
			"2023-11-14T22:13:20.000000000Z plain line\n" +
				`2023-11-14T22:13:21.000000000Z {"level":"warn","msg":"slow"}` + "\n"))
	}))
	defer srv.Close()

	c := NewKubeClient(srv.URL, http.Header{"Authorization": {"Bearer user-jwt"}})
	entries, err := c.Query(context.Background(), &QueryParams{
		ClusterID: "cluster-1",
		Namespace: "prod",
		Pod:       "api-1",
		Container: "app",
	})
	require.NoError(t, err)

	assert.Equal(t, "/clusters/cluster-1/api/v1/namespaces/prod/pods/api-1/log", gotPath)
	assert.Equal(t, "Bearer user-jwt", gotAuth)
	assert.Contains(t, gotQuery, "timestamps=true")
	assert.Contains(t, gotQuery, "container=app")
	require.Len(t, entries, 2)
	// Newest first: the warn line is newer.
	assert.Equal(t, "slow", entries[0].Message)
	assert.Equal(t, "WARN", entries[0].Level)
	assert.Equal(t, "plain line", entries[1].Message)
	assert.Equal(t, "INFO", entries[1].Level)
	assert.Equal(t, "prod", entries[0].Namespace)
	assert.Equal(t, "api-1", entries[0].Pod)
}

func TestKubeClient_QueryRequiresPod(t *testing.T) {
	c := NewKubeClient("http://example", nil)
	_, err := c.Query(context.Background(), &QueryParams{ClusterID: "c", Namespace: "prod"})
	require.ErrorIs(t, err, ErrPodRequired)
}

func TestKubeClient_QueryHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("nope"))
	}))
	defer srv.Close()

	c := NewKubeClient(srv.URL, nil)
	_, err := c.Query(context.Background(), &QueryParams{ClusterID: "c", Namespace: "prod", Pod: "api-1"})
	require.Error(t, err, "expected error on 403")
}

// A follow stream is long-lived by design, so it must not run on the client
// whose Timeout also bounds body reads — that cut every tail at 30s.
func TestKubeClient_TailOutlivesQueryTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte("2023-11-14T22:13:20.000000000Z late line\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	c := NewKubeClient(srv.URL, nil)
	require.Zero(t, c.followClient.Timeout, "follow client must not carry a whole-response deadline")
	// Shrink the one-shot budget far below the server's write delay: if the
	// follow path used that client, the read would be cut before the line lands.
	c.httpClient.Timeout = 20 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch, err := c.Tail(ctx, &QueryParams{ClusterID: "c", Namespace: "prod", Pod: "api-1"})
	require.NoError(t, err)

	select {
	case ev, ok := <-ch:
		require.True(t, ok, "stream closed before delivering the line")
		require.NoError(t, ev.Err)
		assert.Equal(t, "late line", ev.Entry.Message)
	case <-ctx.Done():
		t.Fatal("timed out waiting for the tailed line")
	}
}

// A truncated stream must arrive as a terminal error, not as a bare channel
// close that the RPC would report as a healthy end of stream.
func TestKubeClient_TailSurfacesReadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)
		// Announce more body than we send, then cut the connection: the
		// scanner fails mid-read instead of seeing a clean EOF.
		w.Header().Set("Content-Length", "512")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("2023-11-14T22:13:20.000000000Z first line\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	c := NewKubeClient(srv.URL, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch, err := c.Tail(ctx, &QueryParams{ClusterID: "c", Namespace: "prod", Pod: "api-1"})
	require.NoError(t, err)

	var got []TailEvent
	for ev := range ch {
		got = append(got, ev)
	}
	require.NotEmpty(t, got)
	last := got[len(got)-1]
	require.Error(t, last.Err, "expected the truncated stream to end with an error event")
}
