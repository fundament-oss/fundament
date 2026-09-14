package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetVersionTime(t *testing.T) {
	publishTime := time.Date(2024, 6, 1, 10, 30, 0, 0, time.UTC)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/github.com/example/mod/@v/v1.0.0.info", r.URL.Path)
		_, err := fmt.Fprintf(w, `{"Version":"v1.0.0","Time":"%s"}`, publishTime.Format(time.RFC3339))
		require.NoError(t, err)
	}))
	defer srv.Close()

	client := NewClient(Options{RetryDelay: time.Millisecond})
	got, err := client.GetVersionTime(context.Background(), srv.URL, "github.com/example/mod", "v1.0.0")
	require.NoError(t, err)
	assert.Equal(t, publishTime, got)
}

func TestGetVersionTime_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewClient(Options{RetryDelay: time.Millisecond})
	_, err := client.GetVersionTime(context.Background(), srv.URL, "github.com/example/mod", "v1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetVersionTime_NoPublishTime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := fmt.Fprint(w, `{"Version":"v1.0.0"}`)
		require.NoError(t, err)
	}))
	defer srv.Close()

	client := NewClient(Options{RetryDelay: time.Millisecond})
	_, err := client.GetVersionTime(context.Background(), srv.URL, "github.com/example/mod", "v1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no publish time")
}

func TestGetVersionTime_UppercasePath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// MyOrg should be encoded as !my!org
		assert.Equal(t, "/github.com/!my!org/mod/@v/v1.0.0.info", r.URL.Path)
		_, err := fmt.Fprintf(w, `{"Version":"v1.0.0","Time":"2024-01-01T00:00:00Z"}`)
		require.NoError(t, err)
	}))
	defer srv.Close()

	client := NewClient(Options{RetryDelay: time.Millisecond})
	_, err := client.GetVersionTime(context.Background(), srv.URL, "github.com/MyOrg/mod", "v1.0.0")
	require.NoError(t, err)
}

func TestGetVersionTime_ProxyChainComma_StopsOn500(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		_, err := fmt.Fprint(w, `{"Version":"v1.0.0","Time":"2024-01-01T00:00:00Z"}`)
		require.NoError(t, err)
	}))
	defer srv2.Close()

	// Comma-separated: 500 should NOT fall through
	proxyURL := srv.URL + "," + srv2.URL
	client := NewClient(Options{RetryDelay: time.Millisecond})
	_, err := client.GetVersionTime(context.Background(), proxyURL, "github.com/example/mod", "v1.0.0")
	require.Error(t, err)
	assert.Equal(t, defaultMaxAttempts, callCount, "should retry the first proxy but not fall through to the second on 500")
}

func TestGetVersionTime_ProxyChainComma_FallsThrough404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := fmt.Fprint(w, `{"Version":"v1.0.0","Time":"2024-01-01T00:00:00Z"}`)
		require.NoError(t, err)
	}))
	defer srv2.Close()

	proxyURL := srv.URL + "," + srv2.URL
	client := NewClient(Options{RetryDelay: time.Millisecond})
	got, err := client.GetVersionTime(context.Background(), proxyURL, "github.com/example/mod", "v1.0.0")
	require.NoError(t, err)
	assert.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), got)
}

func TestGetVersionTime_ProxyChainPipe_FallsThroughOn500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := fmt.Fprint(w, `{"Version":"v1.0.0","Time":"2024-01-01T00:00:00Z"}`)
		require.NoError(t, err)
	}))
	defer srv2.Close()

	// Pipe-separated: 500 SHOULD fall through
	proxyURL := srv.URL + "|" + srv2.URL
	client := NewClient(Options{RetryDelay: time.Millisecond})
	got, err := client.GetVersionTime(context.Background(), proxyURL, "github.com/example/mod", "v1.0.0")
	require.NoError(t, err)
	assert.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), got)
}

func TestEncodePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"github.com/example/mod", "github.com/example/mod"},
		{"github.com/MyOrg/Mod", "github.com/!my!org/!mod"},
		{"github.com/Azure/azure-sdk", "github.com/!azure/azure-sdk"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, EncodePath(tt.input))
		})
	}
}

func TestGetVersionTime_RetriesTransient500(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		if callCount < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, err := fmt.Fprint(w, `{"Version":"v1.0.0","Time":"2024-01-01T00:00:00Z"}`)
		require.NoError(t, err)
	}))
	defer srv.Close()

	client := NewClient(Options{RetryDelay: time.Millisecond})
	got, err := client.GetVersionTime(context.Background(), srv.URL, "github.com/example/mod", "v1.0.0")
	require.NoError(t, err)
	assert.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), got)
	assert.Equal(t, 3, callCount)
}

func TestGetVersionTime_RetriesTransportError(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		// Hang up without a response to simulate a flaky connection.
		hj, ok := w.(http.Hijacker)
		require.True(t, ok)
		conn, _, err := hj.Hijack()
		require.NoError(t, err)
		_ = conn.Close()
	}))
	defer srv.Close()

	client := NewClient(Options{RetryDelay: time.Millisecond})
	_, err := client.GetVersionTime(context.Background(), srv.URL, "github.com/example/mod", "v1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("after %d attempts", defaultMaxAttempts))
	assert.Equal(t, defaultMaxAttempts, callCount)
}

func TestGetVersionTime_DoesNotRetryNotFound(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewClient(Options{RetryDelay: time.Millisecond})
	_, err := client.GetVersionTime(context.Background(), srv.URL, "github.com/example/mod", "v1.0.0")
	require.Error(t, err)
	assert.Equal(t, 1, callCount, "404 is final, not transient")
}

func TestGetVersionTime_StopsRetryingOnCancelledContext(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	client := NewClient(Options{RetryDelay: time.Hour})
	cancel()

	_, err := client.GetVersionTime(ctx, srv.URL, "github.com/example/mod", "v1.0.0")
	require.Error(t, err)
	assert.LessOrEqual(t, callCount, 1)
}
