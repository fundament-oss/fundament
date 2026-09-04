package prometheus

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const vectorBody = `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"node":"n1"},"value":[1700000000,"1.5"]}]}}`

const matrixBody = `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[[1700000000,"2.5"]]}]}}`

func TestHTTPClientWithAuth_SendsBasicAuth(t *testing.T) {
	var gotUser, gotPass string
	var gotOK bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, gotOK = r.BasicAuth()
		switch r.URL.Path {
		case "/api/v1/query":
			w.Write([]byte(vectorBody))
		case "/api/v1/query_range":
			w.Write([]byte(matrixBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewHTTPClientWithAuth(srv.URL, "admin", "secret")

	samples, err := c.Query(context.Background(), "up", time.Now())
	require.NoError(t, err)
	require.Len(t, samples, 1)
	assert.Equal(t, 1.5, samples[0].Value)
	require.True(t, gotOK)
	assert.Equal(t, "admin", gotUser)
	assert.Equal(t, "secret", gotPass)

	series, err := c.QueryRange(context.Background(), "up", time.Now().Add(-time.Hour), time.Now(), time.Minute)
	require.NoError(t, err)
	require.Len(t, series, 1)
	require.True(t, gotOK)
	assert.Equal(t, "admin", gotUser)
}

func TestHTTPClient_NoAuthHeaderWithoutCredentials(t *testing.T) {
	var sawAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _, sawAuth = r.BasicAuth()
		w.Write([]byte(vectorBody))
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL)
	_, err := c.Query(context.Background(), "up", time.Now())
	require.NoError(t, err)
	assert.False(t, sawAuth)
}

func TestHTTPClient_PathPrefixPreserved(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(vectorBody))
	}))
	defer srv.Close()

	c := NewHTTPClientWithAuth(srv.URL+"/api/datasources/proxy/uid/abc", "u", "p")
	_, err := c.Query(context.Background(), "up", time.Now())
	require.NoError(t, err)
	assert.Equal(t, "/api/datasources/proxy/uid/abc/api/v1/query", gotPath)
}

func TestHTTPClient_StatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewHTTPClientWithAuth(srv.URL, "u", "stale")
	_, err := c.Query(context.Background(), "up", time.Now())
	require.Error(t, err)

	var statusErr *StatusError
	require.True(t, errors.As(err, &statusErr))
	assert.Equal(t, http.StatusUnauthorized, statusErr.StatusCode)
}

func TestHTTPClient_WithTransport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(vectorBody))
	}))
	defer srv.Close()

	var used bool
	rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		used = true
		return http.DefaultTransport.RoundTrip(r)
	})

	c := NewHTTPClientWithAuth(srv.URL, "u", "p", WithTransport(rt))
	_, err := c.Query(context.Background(), "up", time.Now())
	require.NoError(t, err)
	assert.True(t, used)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestHTTPClient_RetriesOnceOnConnectionReset(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			// Tear the connection down mid-flight so the client sees a
			// transport-level error rather than an HTTP status.
			hj, ok := w.(http.Hijacker)
			require.True(t, ok)
			conn, _, err := hj.Hijack()
			require.NoError(t, err)
			conn.Close()
			return
		}
		w.Write([]byte(vectorBody))
	}))
	defer srv.Close()

	c := NewHTTPClientWithAuth(srv.URL, "u", "p")
	samples, err := c.Query(context.Background(), "up", time.Now())
	require.NoError(t, err, "one torn connection must not fail the query")
	require.Len(t, samples, 1)
	assert.Equal(t, int32(2), calls.Load())
}
