package logs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			want: `{namespace_name="prod"} |= "timeout"`,
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
	assert.Equal(t, `{namespace_name="prod"} |= "boom"`, gotQuery.Get("query"))
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
		assert.Equal(t, want, normalizeLevel(in), "normalizeLevel(%q)", in)
	}
}
