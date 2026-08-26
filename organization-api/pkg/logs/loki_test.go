package logs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A tail that polls into a 401 must end with an error: the credentials rotated,
// no further poll can succeed, and the caller needs the signal to re-resolve.
// Silently continuing left the stream open and permanently empty.
func TestLokiClient_TailSurfacesUnauthorized(t *testing.T) {
	var polls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		polls++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewLokiClientWithAuth(srv.URL, "user", "stale-password")
	c.pollInterval = 10 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch, err := c.Tail(ctx, &QueryParams{ClusterID: "c"})
	require.NoError(t, err)

	select {
	case ev, ok := <-ch:
		require.True(t, ok, "stream closed without reporting the failure")
		require.Error(t, ev.Err)
		var statusErr *StatusError
		require.ErrorAs(t, ev.Err, &statusErr)
		assert.Equal(t, http.StatusUnauthorized, statusErr.StatusCode)
		assert.Equal(t, 1, polls, "401 should terminate on the first poll, not after retries")
	case <-ctx.Done():
		t.Fatal("timed out waiting for the terminal error event")
	}
}

// Transient failures must not tear down the tail — only a persistent run does.
func TestLokiClient_TailToleratesTransientFailure(t *testing.T) {
	var polls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		polls++
		if polls == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		ts := time.Now().Add(time.Second).UnixNano()
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[{"stream":{"` +
			labelNamespace + `":"kube-system","` + labelPod + `":"api-1"},"values":[["` +
			strconv.FormatInt(ts, 10) + `","hello"]]}]}}`))
		_ = r
	}))
	defer srv.Close()

	c := NewLokiClient(srv.URL)
	c.pollInterval = 10 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch, err := c.Tail(ctx, &QueryParams{ClusterID: "c"})
	require.NoError(t, err)

	select {
	case ev, ok := <-ch:
		require.True(t, ok, "stream closed instead of recovering from one 502")
		require.NoError(t, ev.Err, "a single failed poll should not end the tail")
		assert.Equal(t, "hello", ev.Entry.Message)
	case <-ctx.Done():
		t.Fatal("timed out waiting for an entry after the transient failure")
	}
}

func TestBuildLogQL(t *testing.T) {
	tests := []struct {
		name string
		p    QueryParams
		want string
	}{
		{
			name: "no filters defaults to non-empty selector",
			p:    QueryParams{},
			want: `{namespace_name=~".+"}`,
		},
		{
			name: "namespace only",
			p:    QueryParams{Namespace: "prod"},
			want: `{namespace_name="prod"}`,
		},
		{
			name: "namespace pod container",
			p:    QueryParams{Namespace: "prod", Pod: "api-1", Container: "app"},
			want: `{namespace_name="prod", pod_name="api-1", container_name="app"}`,
		},
		{
			name: "search adds line filter",
			p:    QueryParams{Namespace: "prod", Search: "timeout"},
			want: `{namespace_name="prod"} |~ "(?i)timeout"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, buildLogQL(&tc.p))
		})
	}
}

func TestLokiClient_Query(t *testing.T) {
	const body = `{
	  "status": "success",
	  "data": {
	    "resultType": "streams",
	    "result": [
	      {
	        "stream": {"namespace_name": "prod", "pod_name": "api-1", "container_name": "app"},
	        "values": [
	          ["1700000000000000000", "{\"level\":\"error\",\"msg\":\"boom\",\"code\":500}"],
	          ["1700000001000000000", "plain info line"]
	        ]
	      }
	    ]
	  }
	}`

	var gotPath string
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewLokiClient(srv.URL)
	entries, err := c.Query(context.Background(), &QueryParams{
		ClusterID: "cluster-1",
		Namespace: "prod",
		Search:    "boom",
	})
	require.NoError(t, err)

	assert.Equal(t, "/vali/api/v1/query_range", gotPath)
	assert.Equal(t, `{namespace_name="prod"} |~ "(?i)boom"`, gotQuery.Get("query"))
	assert.Equal(t, "backward", gotQuery.Get("direction"))
	require.Len(t, entries, 2)
	// Newest first.
	assert.True(t, entries[0].Timestamp.After(entries[1].Timestamp))

	// The JSON line should be parsed.
	var jsonEntry *Entry
	for i := range entries {
		if entries[i].Message == "boom" {
			jsonEntry = &entries[i]
		}
	}
	require.NotNil(t, jsonEntry, "did not find parsed JSON entry")
	assert.Equal(t, "ERROR", jsonEntry.Level)
	assert.Equal(t, "prod", jsonEntry.Namespace)
	assert.Equal(t, "api-1", jsonEntry.Pod)
	assert.Equal(t, "app", jsonEntry.Container)
	assert.Equal(t, "500", jsonEntry.Fields["code"])
}

func TestLokiClient_QueryHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewLokiClient(srv.URL)
	_, err := c.Query(context.Background(), &QueryParams{Namespace: "x", End: time.Now()})
	require.Error(t, err, "expected error on 500 response")
}

func TestLokiClient_Labels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/vali/api/v1/label/namespace_name/values":
			_, _ = w.Write([]byte(`{"status":"success","data":["prod","staging"]}`))
		case "/vali/api/v1/label/pod_name/values":
			_, _ = w.Write([]byte(`{"status":"success","data":["api-1"]}`))
		default:
			_, _ = w.Write([]byte(`{"status":"success","data":[]}`))
		}
	}))
	defer srv.Close()

	c := NewLokiClient(srv.URL)
	labels, err := c.Labels(context.Background(), "cluster-1", "prod", time.Time{}, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, []string{"prod", "staging"}, labels.Namespaces)
	assert.Equal(t, []string{"api-1"}, labels.Pods)
}

func TestLokiClient_BasicAuth(t *testing.T) {
	const respBody = `{"status":"success","data":{"resultType":"streams","result":[]}}`

	t.Run("with credentials", func(t *testing.T) {
		var gotUser, gotPass string
		var gotOK bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotUser, gotPass, gotOK = r.BasicAuth()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(respBody))
		}))
		defer srv.Close()

		c := NewLokiClientWithAuth(srv.URL, "observer", "s3cr3t")
		_, err := c.Query(context.Background(), &QueryParams{Namespace: "prod"})
		require.NoError(t, err)
		require.True(t, gotOK, "expected basic auth header")
		assert.Equal(t, "observer", gotUser)
		assert.Equal(t, "s3cr3t", gotPass)
	})

	t.Run("without credentials", func(t *testing.T) {
		var gotOK bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _, gotOK = r.BasicAuth()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(respBody))
		}))
		defer srv.Close()

		c := NewLokiClient(srv.URL)
		_, err := c.Query(context.Background(), &QueryParams{Namespace: "prod"})
		require.NoError(t, err)
		assert.False(t, gotOK, "expected no Authorization header")
	})
}

func TestNormalizeLevel(t *testing.T) {
	tests := map[string]string{
		"error":   "ERROR",
		"ERR":     "ERROR",
		"fatal":   "ERROR",
		"warning": "WARN",
		"warn":    "WARN",
		"info":    "INFO",
		"notice":  "INFO",
		"debug":   "DEBUG",
		"trace":   "DEBUG",
		"":        "",
		"weird":   "",
	}
	for in, want := range tests {
		assert.Equal(t, want, NormalizeLevel(in), "NormalizeLevel(%q)", in)
	}
}

func TestLevelPreFilter(t *testing.T) {
	tests := []struct {
		name   string
		levels []string
		want   string
	}{
		{"no filter", nil, ""},
		{"error only", []string{"ERROR"}, "(?i)alert|crit|emerg|err|fatal|panic"},
		{"error and warn", []string{"ERROR", "WARN"}, "(?i)alert|crit|emerg|err|fatal|panic|warn"},
		{"debug only", []string{"DEBUG"}, "(?i)debug|trace"},
		// An unclassifiable line is reported as the default level, and carries no
		// level token — so a set including it cannot be narrowed without losing
		// exactly those lines.
		{"including the default level does not narrow", []string{"ERROR", "INFO"}, ""},
		{"every level does not narrow", []string{"ERROR", "WARN", "INFO", "DEBUG"}, ""},
		{"unrecognised level is ignored", []string{"ERROR", "bogus"}, "(?i)alert|crit|emerg|err|fatal|panic"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, levelPreFilter(tt.levels))
		})
	}
}

// The pre-filter narrows what Vali scans; it must never be the thing that
// decides the result, so it is always a superset of the exact filter.
func TestLevelPreFilterIsSupersetOfExactFilter(t *testing.T) {
	entries := []Entry{
		{Level: "ERROR", Message: `{"level":"error","msg":"boom"}`},
		{Level: "WARN", Message: `{"level":"warn","msg":"careful"}`},
		{Level: "INFO", Message: "plain text line with no level at all"},
	}
	for _, e := range entries {
		if NormalizeLevel(e.Level) != "ERROR" {
			continue
		}
		pattern := levelPreFilter([]string{"ERROR"})
		require.NotEmpty(t, pattern)
		re, err := regexp.Compile(pattern)
		require.NoError(t, err)
		assert.True(t, re.MatchString(e.Message),
			"pre-filter %q must not exclude an ERROR line: %q", pattern, e.Message)
	}
}

func TestBuildLogQLLevelFilter(t *testing.T) {
	got := buildLogQL(&QueryParams{Namespace: "prod", Levels: []string{"ERROR"}})
	assert.Equal(t, `{namespace_name="prod"} |~ "(?i)alert|crit|emerg|err|fatal|panic"`, got)

	// Search and levels compose as two pipeline stages.
	got = buildLogQL(&QueryParams{Namespace: "prod", Search: "boom", Levels: []string{"ERROR"}})
	assert.Equal(t, `{namespace_name="prod"} |~ "(?i)boom" |~ "(?i)alert|crit|emerg|err|fatal|panic"`, got)
}

// A regression guard for the quoting: %q keeps a hostile value inside the
// string literal. If this ever became %s, injection would be possible and the
// alphanumeric-only cases elsewhere would still pass.
func TestBuildLogQLQuotesHostileValues(t *testing.T) {
	got := buildLogQL(&QueryParams{
		Namespace: `a" , container_name=~".+" }|~"`,
		Search:    "\\\" x",
	})
	assert.Equal(t, 1, strings.Count(got, "{"), "selector must not be broken out of: %s", got)
	assert.Contains(t, got, `namespace_name="a\" , container_name=~\".+\" }|~\""`)
}

// A malformed timestamp used to be discarded into time.Unix(0,0), which the
// tail then dropped as "at or before the watermark" — a Vali format change
// would have produced a permanently empty, permanently error-free stream.
func TestStreamsToEntriesRejectsBadTimestamp(t *testing.T) {
	_, err := streamsToEntries([]lokiStream{{
		Stream: map[string]string{labelNamespace: "prod"},
		Values: [][2]string{{"not-a-timestamp", "hello"}},
	}}, "cluster-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse entry timestamp")
}
