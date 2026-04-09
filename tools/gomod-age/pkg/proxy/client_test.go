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
		fmt.Fprintf(w, `{"Version":"v1.0.0","Time":"%s"}`, publishTime.Format(time.RFC3339))
	}))
	defer srv.Close()

	client := NewClient(nil)
	got, err := client.GetVersionTime(context.Background(), srv.URL, "github.com/example/mod", "v1.0.0")
	require.NoError(t, err)
	assert.Equal(t, publishTime, got)
}

func TestGetVersionTime_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewClient(nil)
	_, err := client.GetVersionTime(context.Background(), srv.URL, "github.com/example/mod", "v1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetVersionTime_NoPublishTime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"Version":"v1.0.0"}`)
	}))
	defer srv.Close()

	client := NewClient(nil)
	_, err := client.GetVersionTime(context.Background(), srv.URL, "github.com/example/mod", "v1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no publish time")
}

func TestGetVersionTime_UppercasePath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// MyOrg should be encoded as !my!org
		assert.Equal(t, "/github.com/!my!org/mod/@v/v1.0.0.info", r.URL.Path)
		fmt.Fprintf(w, `{"Version":"v1.0.0","Time":"2024-01-01T00:00:00Z"}`)
	}))
	defer srv.Close()

	client := NewClient(nil)
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
		fmt.Fprint(w, `{"Version":"v1.0.0","Time":"2024-01-01T00:00:00Z"}`)
	}))
	defer srv2.Close()

	// Comma-separated: 500 should NOT fall through
	proxyURL := srv.URL + "," + srv2.URL
	client := NewClient(nil)
	_, err := client.GetVersionTime(context.Background(), proxyURL, "github.com/example/mod", "v1.0.0")
	require.Error(t, err)
	assert.Equal(t, 1, callCount, "should not fall through to second proxy on 500")
}

func TestGetVersionTime_ProxyChainComma_FallsThrough404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"Version":"v1.0.0","Time":"2024-01-01T00:00:00Z"}`)
	}))
	defer srv2.Close()

	proxyURL := srv.URL + "," + srv2.URL
	client := NewClient(nil)
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
		fmt.Fprint(w, `{"Version":"v1.0.0","Time":"2024-01-01T00:00:00Z"}`)
	}))
	defer srv2.Close()

	// Pipe-separated: 500 SHOULD fall through
	proxyURL := srv.URL + "|" + srv2.URL
	client := NewClient(nil)
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
