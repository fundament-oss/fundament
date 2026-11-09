package provider

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	marketplacev1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/marketplace/v1"
	catalogv1 "github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/proto/gen/catalog/v1"
	"github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/proto/gen/install/v1/installv1connect"
)

const (
	acmeGrafanaID  = "019b4000-0000-7000-8000-00000000000a"
	otherGrafanaID = "019b4000-0000-7000-8000-00000000000b"
)

// fakeInstall serves two "grafana" listings from different publishers.
type fakeInstall struct {
	installv1connect.UnimplementedInstallServiceHandler
	versions map[string][]*catalogv1.PublishedVersion
}

func (fakeInstall) ListPlugins(context.Context, *catalogv1.ListPluginsRequest) (*catalogv1.ListPluginsResponse, error) {
	return catalogv1.ListPluginsResponse_builder{Plugins: []*catalogv1.PluginSummary{
		catalogv1.PluginSummary_builder{Id: otherGrafanaID, Name: "grafana", OrganizationId: "org-b"}.Build(),
		catalogv1.PluginSummary_builder{Id: acmeGrafanaID, Name: "grafana", OrganizationId: "org-a"}.Build(),
	}}.Build(), nil
}

func (fakeInstall) ListPublishers(context.Context, *catalogv1.ListPublishersRequest) (*catalogv1.ListPublishersResponse, error) {
	return catalogv1.ListPublishersResponse_builder{Publishers: []*marketplacev1.Publisher{
		marketplacev1.Publisher_builder{Id: "org-a", Name: "acme"}.Build(),
		marketplacev1.Publisher_builder{Id: "org-b", Name: "other"}.Build(),
	}}.Build(), nil
}

func (f fakeInstall) ListPluginVersions(_ context.Context, req *catalogv1.ListPluginVersionsRequest) (*catalogv1.ListPluginVersionsResponse, error) {
	return catalogv1.ListPluginVersionsResponse_builder{Versions: f.versions[req.GetPluginId()]}.Build(), nil
}

func resourceWithInstall(t *testing.T, fake fakeInstall) *PluginInstallationResource {
	t.Helper()
	_, handler := installv1connect.NewInstallServiceHandler(fake)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &PluginInstallationResource{client: &FundamentClient{
		InstallService: installv1connect.NewInstallServiceClient(srv.Client(), srv.URL),
	}}
}

func version(v, hash string) *catalogv1.PublishedVersion {
	return catalogv1.PublishedVersion_builder{Version: v, DefinitionHash: hash}.Build()
}

func TestResolveLatestDefinition_MatchesPublisherAndName(t *testing.T) {
	r := resourceWithInstall(t, fakeInstall{versions: map[string][]*catalogv1.PublishedVersion{
		acmeGrafanaID:  {version("2.0.0", "sha256:b"), version("1.0.0", "sha256:a")},
		otherGrafanaID: {version("9.0.0", "sha256:other")},
	}})

	v, hash, err := r.resolveLatestDefinition(context.Background(), "acme", "grafana")
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", v)
	assert.Equal(t, "sha256:b", hash)
}

func TestResolveLatestDefinition_NotFound(t *testing.T) {
	r := resourceWithInstall(t, fakeInstall{})

	_, _, err := r.resolveLatestDefinition(context.Background(), "acme", "loki")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in the Fundament catalog")
	// Draft-only plugins are not listed either; the message must say how to install one.
	assert.Contains(t, err.Error(), "pin plugin_version and definition_hash")
}

func TestResolveLatestDefinition_NoPublishedVersion(t *testing.T) {
	r := resourceWithInstall(t, fakeInstall{})

	_, _, err := r.resolveLatestDefinition(context.Background(), "acme", "grafana")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no published definition to install")
}

func TestResolveLatestDefinition_RequiresInstallURL(t *testing.T) {
	r := &PluginInstallationResource{client: &FundamentClient{}}

	_, _, err := r.resolveLatestDefinition(context.Background(), "acme", "grafana")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "marketplace_install_url")
	assert.Contains(t, err.Error(), "FUNDAMENT_MARKETPLACE_INSTALL_URL")
}
