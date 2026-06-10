package logs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKubeClient_Query(t *testing.T) {
	var gotPath, gotAuth string
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
		fmt.Fprint(w,
			"2023-11-14T22:13:20.000000000Z plain line\n"+
				`2023-11-14T22:13:21.000000000Z {"level":"warn","msg":"slow"}`+"\n")
	}))
	defer srv.Close()

	c := NewKubeClient(srv.URL, "user-jwt")
	entries, err := c.Query(context.Background(), QueryParams{
		ClusterID: "cluster-1",
		Namespace: "prod",
		Pod:       "api-1",
		Container: "app",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if gotPath != "/clusters/cluster-1/api/v1/namespaces/prod/pods/api-1/log" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer user-jwt" {
		t.Errorf("auth = %q", gotAuth)
	}
	if !strings.Contains(gotQuery, "timestamps=true") || !strings.Contains(gotQuery, "container=app") {
		t.Errorf("query = %q", gotQuery)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	// Newest first: the warn line is newer.
	if entries[0].Message != "slow" || entries[0].Level != "WARN" {
		t.Errorf("entries[0] = %+v", entries[0])
	}
	if entries[1].Message != "plain line" || entries[1].Level != "INFO" {
		t.Errorf("entries[1] = %+v", entries[1])
	}
	if entries[0].Namespace != "prod" || entries[0].Pod != "api-1" {
		t.Errorf("labels not set: %+v", entries[0])
	}
}

func TestKubeClient_QueryRequiresPod(t *testing.T) {
	c := NewKubeClient("http://example", "tok")
	_, err := c.Query(context.Background(), QueryParams{ClusterID: "c", Namespace: "prod"})
	if !errors.Is(err, ErrPodRequired) {
		t.Fatalf("err = %v, want ErrPodRequired", err)
	}
}

func TestKubeClient_QueryHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "nope")
	}))
	defer srv.Close()

	c := NewKubeClient(srv.URL, "tok")
	_, err := c.Query(context.Background(), QueryParams{ClusterID: "c", Namespace: "prod", Pod: "api-1"})
	if err == nil {
		t.Fatal("expected error on 403")
	}
}
