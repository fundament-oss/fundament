package assets

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeKubeconfigSource struct {
	host string
}

func (f *fakeKubeconfigSource) HTTPClientFor(_ context.Context, _ string) (http.RoundTripper, string, error) {
	return http.DefaultTransport, f.host, nil
}

// The upstream URL is a contract with plugin-controller: it names the
// namespace AND the Service plugin-<name> (there is no Service "runtime").
func TestPodFetcher_TargetsPluginService(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html></html>"))
	}))
	defer upstream.Close()

	f := &PodFetcher{AdminKubeconfig: &fakeKubeconfigSource{host: upstream.URL}}
	body, contentType, err := f.Fetch(context.Background(), uuid.New(), "cert-manager", "index.html")
	require.NoError(t, err)

	assert.Equal(t,
		"/api/v1/namespaces/plugin-cert-manager/services/plugin-cert-manager:8080/proxy/console/index.html",
		gotPath)
	assert.Equal(t, "text/html", contentType)
	assert.Equal(t, []byte("<html></html>"), body)
}
