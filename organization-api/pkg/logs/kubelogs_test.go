package logs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
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

func TestKubeClient_QueryRequiresNamespace(t *testing.T) {
	c := NewKubeClient("http://example", nil)
	_, err := c.Query(context.Background(), &QueryParams{ClusterID: "c", Pod: "api-1"})
	require.ErrorIs(t, err, ErrNamespaceRequired)
	_, err = c.Tail(context.Background(), &QueryParams{ClusterID: "c"})
	require.ErrorIs(t, err, ErrNamespaceRequired)
}

// fanOutServer serves a namespace listing and a log per pod and container.
// "web-0" has two containers, "web-1" one, and "web-2" is still Pending, so
// its log must not be asked for.
func fanOutServer(t *testing.T, logs map[string]string) (*httptest.Server, *[]string) {
	t.Helper()
	var asked []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/clusters/c/api/v1/namespaces/plugin-cert-manager/pods" {
			_, _ = w.Write([]byte(`{"items":[
				{"metadata":{"name":"web-0"},"spec":{"containers":[{"name":"app"},{"name":"sidecar"}]},"status":{"phase":"Running"}},
				{"metadata":{"name":"web-1"},"spec":{"containers":[{"name":"app"}]},"status":{"phase":"Running"}},
				{"metadata":{"name":"web-2"},"spec":{"containers":[{"name":"app"}]},"status":{"phase":"Pending"}}
			]}`))
			return
		}
		pod := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/clusters/c/api/v1/namespaces/plugin-cert-manager/pods/"), "/log")
		key := pod + "/" + r.URL.Query().Get("container")
		mu.Lock()
		asked = append(asked, key)
		mu.Unlock()
		body, ok := logs[key]
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &asked
}

func TestKubeClient_QueryWithoutPodReadsEveryPod(t *testing.T) {
	srv, asked := fanOutServer(t, map[string]string{
		"web-0/app":     "2023-11-14T22:13:20Z first\n2023-11-14T22:13:23Z fourth\n",
		"web-0/sidecar": "2023-11-14T22:13:22Z third\n",
		"web-1/app":     "2023-11-14T22:13:21Z second\n2023-11-14T22:13:24Z fifth\n",
	})

	c := NewKubeClient(srv.URL, nil)
	entries, err := c.Query(context.Background(), &QueryParams{ClusterID: "c", Namespace: "plugin-cert-manager", Limit: 4})
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"web-0/app", "web-0/sidecar", "web-1/app"}, *asked)
	// Merged newest first across pods, then cut to the limit.
	messages := make([]string, 0, len(entries))
	for _, e := range entries {
		messages = append(messages, e.Message)
	}
	assert.Equal(t, []string{"fifth", "fourth", "third", "second"}, messages)
	assert.Equal(t, "web-1", entries[0].Pod)
	assert.Equal(t, "sidecar", entries[2].Container)
}

func TestKubeClient_QueryWithoutPodNarrowsToContainer(t *testing.T) {
	srv, asked := fanOutServer(t, map[string]string{
		"web-0/app": "2023-11-14T22:13:20Z one\n",
		"web-1/app": "2023-11-14T22:13:21Z two\n",
	})

	c := NewKubeClient(srv.URL, nil)
	entries, err := c.Query(context.Background(), &QueryParams{ClusterID: "c", Namespace: "plugin-cert-manager", Container: "app"})
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"web-0/app", "web-1/app"}, *asked)
	assert.Len(t, entries, 2)
}

// One pod that cannot be read must not hide the others.
func TestKubeClient_QueryWithoutPodSkipsUnreadablePods(t *testing.T) {
	srv, _ := fanOutServer(t, map[string]string{
		"web-1/app": "2023-11-14T22:13:21Z still here\n",
	})

	c := NewKubeClient(srv.URL, nil)
	entries, err := c.Query(context.Background(), &QueryParams{ClusterID: "c", Namespace: "plugin-cert-manager"})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "still here", entries[0].Message)
}

func TestKubeClient_QueryWithoutPodFailsWhenNoPodCanBeRead(t *testing.T) {
	srv, _ := fanOutServer(t, map[string]string{})

	c := NewKubeClient(srv.URL, nil)
	_, err := c.Query(context.Background(), &QueryParams{ClusterID: "c", Namespace: "plugin-cert-manager"})
	var statusErr *StatusError
	require.ErrorAs(t, err, &statusErr)
}

// With a filter, the pods share the MaxLimit fetch rather than each reading
// all of it.
func TestKubeClient_QueryWithoutPodSharesTheFilteredFetch(t *testing.T) {
	var mu sync.Mutex
	var tailLines []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/pods") {
			_, _ = w.Write([]byte(`{"items":[
				{"metadata":{"name":"a"},"spec":{"containers":[{"name":"app"}]},"status":{"phase":"Running"}},
				{"metadata":{"name":"b"},"spec":{"containers":[{"name":"app"}]},"status":{"phase":"Running"}}
			]}`))
			return
		}
		mu.Lock()
		tailLines = append(tailLines, r.URL.Query().Get("tailLines"))
		mu.Unlock()
	}))
	t.Cleanup(srv.Close)

	c := NewKubeClient(srv.URL, nil)
	_, err := c.Query(context.Background(), &QueryParams{ClusterID: "c", Namespace: "ns", Search: "boom", Limit: 50})
	require.NoError(t, err)

	want := strconv.Itoa(MaxLimit / 2)
	assert.Equal(t, []string{want, want}, tailLines)
}

// collectTail reads n entries off a tail, failing the test if they do not come.
func collectTail(t *testing.T, ch <-chan TailEvent, n int) []string {
	t.Helper()
	var messages []string
	for len(messages) < n {
		select {
		case ev, ok := <-ch:
			require.True(t, ok, "tail closed after %v", messages)
			require.NoError(t, ev.Err)
			messages = append(messages, ev.Entry.Message)
		case <-time.After(5 * time.Second):
			require.Failf(t, "tail went quiet", "got %v", messages)
		}
	}
	return messages
}

func TestKubeClient_TailWithoutPodMergesEveryPod(t *testing.T) {
	srv, _ := fanOutServer(t, map[string]string{
		"web-0/app":     "2023-11-14T22:13:20Z from web-0\n",
		"web-0/sidecar": "2023-11-14T22:13:21Z from the sidecar\n",
		"web-1/app":     "2023-11-14T22:13:22Z from web-1\n",
	})

	ctx, cancel := context.WithCancel(context.Background())
	c := NewKubeClient(srv.URL, nil)
	ch, err := c.Tail(ctx, &QueryParams{ClusterID: "c", Namespace: "plugin-cert-manager"})
	require.NoError(t, err)

	messages := collectTail(t, ch, 3)
	assert.ElementsMatch(t, []string{"from web-0", "from the sidecar", "from web-1"}, messages)

	// The tail is about the namespace, so it stays open until the caller ends it.
	// Cancelling closes it without an error.
	cancel()
	for ev := range ch {
		require.NoError(t, ev.Err)
	}
}

// A pod that was still starting when the tail opened is followed once it runs.
func TestKubeClient_TailWithoutPodFollowsPodsThatStartLater(t *testing.T) {
	var mu sync.Mutex
	phase := "Pending"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/pods") {
			mu.Lock()
			current := phase
			phase = "Running"
			mu.Unlock()
			_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"late"},"spec":{"containers":[{"name":"app"}]},"status":{"phase":"` + current + `"}}]}`))
			return
		}
		_, _ = w.Write([]byte("2023-11-14T22:13:20Z up now\n"))
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c := NewKubeClient(srv.URL, nil)
	c.relistInterval = 10 * time.Millisecond
	ch, err := c.Tail(ctx, &QueryParams{ClusterID: "c", Namespace: "ns"})
	require.NoError(t, err)

	assert.Equal(t, []string{"up now"}, collectTail(t, ch, 1))
}

// A listing that fails ends the tail, with the error as its last event.
func TestKubeClient_TailWithoutPodEndsWhenTheListingFails(t *testing.T) {
	var mu sync.Mutex
	listings := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/pods") {
			mu.Lock()
			listings++
			first := listings == 1
			mu.Unlock()
			if !first {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte(`{"items":[]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	c := NewKubeClient(srv.URL, nil)
	c.relistInterval = 10 * time.Millisecond
	ch, err := c.Tail(context.Background(), &QueryParams{ClusterID: "c", Namespace: "ns"})
	require.NoError(t, err)

	var events []TailEvent
	for ev := range ch {
		events = append(events, ev)
	}
	require.Len(t, events, 1)
	var statusErr *StatusError
	require.ErrorAs(t, events[0].Err, &statusErr)
	assert.Equal(t, http.StatusForbidden, statusErr.StatusCode)
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
