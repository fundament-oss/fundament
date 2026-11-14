package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"fundament": providerserver.NewProtocol6WithError(New("test")()),
}

func TestDeriveAuthnEndpoint(t *testing.T) {
	tests := map[string]struct {
		organizationEndpoint string
		expected             string
	}{
		"replaces the host label": {
			organizationEndpoint: "https://organization.example.com",
			expected:             "https://authn.example.com",
		},
		"keeps the path": {
			organizationEndpoint: "http://organization.example.com/api/v1",
			expected:             "http://authn.example.com/api/v1",
		},
		"keeps the query": {
			organizationEndpoint: "http://organization.example.com?foo=bar",
			expected:             "http://authn.example.com?foo=bar",
		},
		"unchanged when organization is not in the host": {
			organizationEndpoint: "http://api.example.com",
			expected:             "http://api.example.com",
		},
		"keeps the port": {
			organizationEndpoint: "http://organization.example.com:9090",
			expected:             "http://authn.example.com:9090",
		},
		"only replaces the first occurrence": {
			organizationEndpoint: "http://organization.organization.example.com",
			expected:             "http://authn.organization.example.com",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.expected, deriveAuthnEndpoint(tt.organizationEndpoint))
		})
	}
}

func TestServiceURLsDiscover(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"issuer": %q,
			"organization_api": "https://organization.example.com",
			"authn_api": "https://authn.example.com",
			"kube_api_proxy": "https://k8s-api.example.com",
			"marketplace_catalog_api": "https://catalog.example.com"
		}`, server.URL)
	}))
	t.Cleanup(server.Close)

	urls := serviceURLs{authn: "https://authn.override.example"}
	require.NoError(t, urls.discover(t.Context(), server.URL))

	assert.Equal(t, serviceURLs{
		organization:       "https://organization.example.com",
		authn:              "https://authn.override.example",
		kubeAPIProxy:       "https://k8s-api.example.com",
		marketplaceInstall: "https://catalog.example.com",
	}, urls)
}

func TestServiceURLsDiscoverWithoutRootURL(t *testing.T) {
	urls := serviceURLs{organization: "https://organization.example.com"}
	require.NoError(t, urls.discover(t.Context(), ""))
	assert.Empty(t, urls.authn, "nothing is derived from the organization endpoint")
}

func TestServiceURLsDiscoverFails(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)

	urls := serviceURLs{}
	require.ErrorContains(t, urls.discover(t.Context(), server.URL), "no discovery document")
}

func TestResolveOptionalURL(t *testing.T) {
	got, err := resolveOptionalURL("https://config.example", "not a url", "FUNDAMENT_X")
	require.NoError(t, err, "config wins; the env value is not read")
	assert.Equal(t, "https://config.example", got)

	got, err = resolveOptionalURL("", "https://env.example", "FUNDAMENT_X")
	require.NoError(t, err)
	assert.Equal(t, "https://env.example", got)

	got, err = resolveOptionalURL("", "", "FUNDAMENT_X")
	require.NoError(t, err)
	assert.Empty(t, got)

	for _, bad := range []string{"marketplace.example", "ftp://x.example", "https://"} {
		_, err = resolveOptionalURL("", bad, "FUNDAMENT_X")
		require.Error(t, err, bad)
		assert.Contains(t, err.Error(), "FUNDAMENT_X")
	}
}
