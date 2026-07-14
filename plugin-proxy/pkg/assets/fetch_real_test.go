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

// fakeUpstream stands in for a shoot API server: it answers the
// PluginInstallation CR read that Fetch performs to pin the version, and
// records the asset path it proxies.
func fakeUpstream(t *testing.T, pluginName, installedVersion string, gotAssetPath *string) *httptest.Server {
	t.Helper()
	crPath := "/apis/plugins.fundament.io/v1/plugininstallations/" + pluginName
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() == crPath {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"apiVersion": "plugins.fundament.io/v1",
				"kind": "PluginInstallation",
				"metadata": {"name": "` + pluginName + `"},
				"spec": {"definitionRef": {"pluginName": "` + pluginName + `", "pluginVersion": "` + installedVersion + `"}}
			}`))
			return
		}
		*gotAssetPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html></html>"))
	}))
}

// The upstream URL is a contract with plugin-controller: it names the
// namespace AND the Service plugin-<name> (there is no Service "runtime"),
// with the http: scheme selector so the API server proxies plain HTTP.
func TestPodFetcher_TargetsPluginService(t *testing.T) {
	var gotPath string
	upstream := fakeUpstream(t, "cert-manager", "1.2.3", &gotPath)
	defer upstream.Close()

	f := &PodFetcher{AdminKubeconfig: &fakeKubeconfigSource{host: upstream.URL}}
	body, contentType, err := f.Fetch(context.Background(), uuid.New(), "cert-manager", "1.2.3", "index.html")
	require.NoError(t, err)

	assert.Equal(t,
		"/api/v1/namespaces/plugin-cert-manager/services/http:plugin-cert-manager:8080/proxy/console/index.html",
		gotPath)
	assert.Equal(t, "text/html", contentType)
	assert.Equal(t, []byte("<html></html>"), body)
}

// A URL whose {version} differs from the installed CR must not serve the
// running pod's bytes — the handler maps ErrVersionMismatch to 404.
func TestPodFetcher_VersionMismatch(t *testing.T) {
	var gotPath string
	upstream := fakeUpstream(t, "cert-manager", "1.2.3", &gotPath)
	defer upstream.Close()

	f := &PodFetcher{AdminKubeconfig: &fakeKubeconfigSource{host: upstream.URL}}
	_, _, err := f.Fetch(context.Background(), uuid.New(), "cert-manager", "9.9.9", "index.html")
	require.ErrorIs(t, err, ErrVersionMismatch)
	assert.Empty(t, gotPath, "asset must not be fetched when the version does not match")
}

// A missing PluginInstallation CR maps to ErrInstallationNotFound (handler: 404).
func TestPodFetcher_InstallationNotFound(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	f := &PodFetcher{AdminKubeconfig: &fakeKubeconfigSource{host: upstream.URL}}
	_, _, err := f.Fetch(context.Background(), uuid.New(), "cert-manager", "1.2.3", "index.html")
	require.ErrorIs(t, err, ErrInstallationNotFound)
}
