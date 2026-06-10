package logs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
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
			want: `{namespace=~".+"}`,
		},
		{
			name: "namespace only",
			p:    QueryParams{Namespace: "prod"},
			want: `{namespace="prod"}`,
		},
		{
			name: "namespace pod container",
			p:    QueryParams{Namespace: "prod", Pod: "api-1", Container: "app"},
			want: `{namespace="prod", pod="api-1", container="app"}`,
		},
		{
			name: "search adds line filter",
			p:    QueryParams{Namespace: "prod", Search: "timeout"},
			want: `{namespace="prod"} |= "timeout"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildLogQL(tc.p); got != tc.want {
				t.Errorf("buildLogQL() = %q, want %q", got, tc.want)
			}
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
	        "stream": {"namespace": "prod", "pod": "api-1", "container": "app"},
	        "values": [
	          ["1700000000000000000", "{\"level\":\"error\",\"msg\":\"boom\",\"code\":500}"],
	          ["1700000001000000000", "plain info line"]
	        ]
	      }
	    ]
	  }
	}`

	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewLokiClient(srv.URL)
	entries, err := c.Query(context.Background(), QueryParams{
		ClusterID: "cluster-1",
		Namespace: "prod",
		Search:    "boom",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if got := gotQuery.Get("query"); got != `{namespace="prod"} |= "boom"` {
		t.Errorf("query param = %q", got)
	}
	if gotQuery.Get("direction") != "backward" {
		t.Errorf("direction = %q, want backward", gotQuery.Get("direction"))
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	// Newest first.
	if !entries[0].Timestamp.After(entries[1].Timestamp) {
		t.Errorf("entries not sorted newest-first")
	}
	// The JSON line should be parsed.
	var jsonEntry *Entry
	for i := range entries {
		if entries[i].Message == "boom" {
			jsonEntry = &entries[i]
		}
	}
	if jsonEntry == nil {
		t.Fatalf("did not find parsed JSON entry")
	}
	if jsonEntry.Level != "ERROR" {
		t.Errorf("level = %q, want ERROR", jsonEntry.Level)
	}
	if jsonEntry.Namespace != "prod" || jsonEntry.Pod != "api-1" || jsonEntry.Container != "app" {
		t.Errorf("stream labels not applied: %+v", jsonEntry)
	}
	if jsonEntry.Fields["code"] != "500" {
		t.Errorf("fields[code] = %q, want 500", jsonEntry.Fields["code"])
	}
}

func TestLokiClient_QueryHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewLokiClient(srv.URL)
	if _, err := c.Query(context.Background(), QueryParams{Namespace: "x", End: time.Now()}); err == nil {
		t.Fatal("expected error on 500 response")
	}
}

func TestLokiClient_Labels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/loki/api/v1/label/namespace/values":
			_, _ = w.Write([]byte(`{"status":"success","data":["prod","staging"]}`))
		case r.URL.Path == "/loki/api/v1/label/pod/values":
			_, _ = w.Write([]byte(`{"status":"success","data":["api-1"]}`))
		default:
			_, _ = w.Write([]byte(`{"status":"success","data":[]}`))
		}
	}))
	defer srv.Close()

	c := NewLokiClient(srv.URL)
	labels, err := c.Labels(context.Background(), "cluster-1", "prod")
	if err != nil {
		t.Fatalf("Labels: %v", err)
	}
	if len(labels.Namespaces) != 2 || labels.Namespaces[0] != "prod" {
		t.Errorf("namespaces = %v", labels.Namespaces)
	}
	if len(labels.Pods) != 1 || labels.Pods[0] != "api-1" {
		t.Errorf("pods = %v", labels.Pods)
	}
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
		if got := normalizeLevel(in); got != want {
			t.Errorf("normalizeLevel(%q) = %q, want %q", in, got, want)
		}
	}
}
