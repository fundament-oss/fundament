package installproxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

type fakeKubeconfigSource struct {
	host string
}

func (f *fakeKubeconfigSource) HTTPClientFor(_ context.Context, _ string) (http.RoundTripper, string, error) {
	return http.DefaultTransport, f.host, nil
}

// The upstream URL is a contract with plugin-controller: the runtime route
// targets the constant-named Service "plugin" in the derived namespace
// (there is no Service "runtime").
func TestClusterProxyBackend_RuntimeTargetsPluginService(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	b := &ClusterProxyBackend{AdminKubeconfig: &fakeKubeconfigSource{host: upstream.URL}}

	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/installations/whatever", http.NoBody)
	w := httptest.NewRecorder()
	b.Serve(w, r, Route{
		Kind:             RouteRuntime,
		ClusterID:        "019f3c89-87ea-722a-967f-146b47bb8549",
		InstallationName: "system--cert-manager",
		RemainingPath:    "api/status",
	})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t,
		"/api/v1/namespaces/plugin-system--cert-manager/services/http:plugin:8080/proxy/api/status",
		gotPath)
}
