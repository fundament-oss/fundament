package logs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

// Unescaped, a pod name containing "?" terminated the path and injected a query
// parameter that survived q.Set — org-api would fetch an attacker-chosen kube
// API path with the caller's credentials attached.
func TestKubeClient_EscapesPathSegments(t *testing.T) {
	var gotPath, gotRawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotRawQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(""))
	}))
	t.Cleanup(srv.Close)

	c := NewKubeClient(srv.URL, http.Header{})
	_, err := c.Query(context.Background(), &QueryParams{
		ClusterID: "cluster-1",
		Namespace: "default",
		Pod:       "nginx?previous=true",
	})
	require.NoError(t, err)

	assert.Equal(t, "/clusters/cluster-1/api/v1/namespaces/default/pods/nginx?previous=true/log", gotPath,
		"the '?' must stay part of the pod name, not start a query string")
	assert.NotContains(t, gotRawQuery, "previous", "no injected query parameter")
}

func TestKubeClient_TraversalStaysInOneSegment(t *testing.T) {
	var gotEscapedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// EscapedPath is what actually went over the wire; r.URL.Path is the
		// decoded form and would show the slashes back.
		gotEscapedPath = r.URL.EscapedPath()
		_, _ = w.Write([]byte(""))
	}))
	t.Cleanup(srv.Close)

	c := NewKubeClient(srv.URL, http.Header{})
	_, err := c.Query(context.Background(), &QueryParams{
		ClusterID: "cluster-1",
		Namespace: "default",
		Pod:       "../../../../api/v1/namespaces/kube-system/secrets",
	})
	require.NoError(t, err)

	// The whole hostile value is one escaped segment: no unescaped separator
	// means no extra path segments for anything downstream to resolve.
	const prefix = "/clusters/cluster-1/api/v1/namespaces/default/pods/"
	assert.Contains(t, gotEscapedPath, "%2F..%2F", "separators must be escaped: %s", gotEscapedPath)
	require.True(t, strings.HasPrefix(gotEscapedPath, prefix),
		"the prefix up to the pod segment must be untouched: %s", gotEscapedPath)
	podSegment := strings.TrimSuffix(strings.TrimPrefix(gotEscapedPath, prefix), "/log")
	assert.NotContains(t, podSegment, "/",
		"the pod segment must carry no unescaped separator: %q", podSegment)
}

// The pod-log endpoint has no server-side search, so the cap used to be applied
// to unfiltered lines: a match older than the last `limit` raw lines was
// reported as "no results".
func TestKubeClient_QueryFiltersSearchAndEnd(t *testing.T) {
	var gotTailLines string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTailLines = r.URL.Query().Get("tailLines")
		_, _ = w.Write([]byte(
			"2026-08-05T12:00:00.000000000Z first noise\n" +
				"2026-08-05T12:00:01.000000000Z the Needle we want\n" +
				"2026-08-05T12:00:02.000000000Z more noise\n" +
				"2026-08-05T13:00:00.000000000Z after the end bound\n"))
	}))
	t.Cleanup(srv.Close)

	c := NewKubeClient(srv.URL, http.Header{})
	entries, err := c.Query(context.Background(), &QueryParams{
		ClusterID: "cluster-1",
		Namespace: "default",
		Pod:       "nginx",
		// Lower-case, against a capitalised line: search is case-insensitive.
		Search: "needle",
		End:    time.Date(2026, 8, 5, 12, 30, 0, 0, time.UTC),
		Limit:  10,
	})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "the Needle we want", entries[0].Message)
	assert.Equal(t, strconv.Itoa(MaxLimit), gotTailLines,
		"a filtered query must over-fetch so the limit applies to matches")
}

func TestKubeClient_QueryHonoursEndBound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(
			"2026-08-05T12:00:00.000000000Z inside\n" +
				"2026-08-05T14:00:00.000000000Z outside\n"))
	}))
	t.Cleanup(srv.Close)

	c := NewKubeClient(srv.URL, http.Header{})
	entries, err := c.Query(context.Background(), &QueryParams{
		ClusterID: "cluster-1",
		Namespace: "default",
		Pod:       "nginx",
		End:       time.Date(2026, 8, 5, 13, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "inside", entries[0].Message)
}

// A plain error here bypassed the degradation classification entirely and
// relayed up to 2KB of the upstream body to the browser.
func TestKubeClient_ReturnsStatusErrorWithoutUpstreamBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"secrets is forbidden for user alice"}`))
	}))
	t.Cleanup(srv.Close)

	c := NewKubeClient(srv.URL, http.Header{})
	_, err := c.Query(context.Background(), &QueryParams{
		ClusterID: "cluster-1", Namespace: "default", Pod: "nginx",
	})
	require.Error(t, err)

	var statusErr *StatusError
	require.ErrorAs(t, err, &statusErr)
	assert.Equal(t, http.StatusForbidden, statusErr.StatusCode)
	assert.NotContains(t, err.Error(), "alice", "the upstream body must not reach the caller")
}
