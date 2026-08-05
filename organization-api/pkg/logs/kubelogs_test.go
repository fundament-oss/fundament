package logs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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

	c := NewKubeClient(srv.URL, "user-jwt")
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
	c := NewKubeClient("http://example", "tok")
	_, err := c.Query(context.Background(), &QueryParams{ClusterID: "c", Namespace: "prod"})
	require.ErrorIs(t, err, ErrPodRequired)
}

func TestKubeClient_QueryHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("nope"))
	}))
	defer srv.Close()

	c := NewKubeClient(srv.URL, "tok")
	_, err := c.Query(context.Background(), &QueryParams{ClusterID: "c", Namespace: "prod", Pod: "api-1"})
	require.Error(t, err, "expected error on 403")
}
