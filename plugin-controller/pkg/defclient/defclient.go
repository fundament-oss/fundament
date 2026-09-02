package defclient

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	catalogv1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/catalog/v1"
	"github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/catalog/v1/catalogv1connect"
)

// Definition is the raw, hash-verifiable manifest returned by the catalog.
type Definition struct {
	Manifest []byte
	Hash     string
}

// Client fetches plugin definitions from marketplace-catalog-api.
type Client interface {
	GetDefinition(ctx context.Context, organizationName, pluginName, pluginVersion string) (Definition, error)
}

type connectClient struct {
	rpc catalogv1connect.CatalogServiceClient
}

// New returns a Client that talks to marketplace-catalog-api at baseURL.
func New(baseURL string, httpClient connect.HTTPClient) Client {
	return &connectClient{rpc: catalogv1connect.NewCatalogServiceClient(httpClient, baseURL)}
}

func (c *connectClient) GetDefinition(ctx context.Context, organizationName, pluginName, pluginVersion string) (Definition, error) {
	// By name rather than by id: a PluginInstallation names the plugin the way
	// it is published, and the controller never sees a catalog id.
	resp, err := c.rpc.GetPluginDefinition(ctx, catalogv1.GetPluginDefinitionRequest_builder{
		Name: catalogv1.PluginRef_builder{
			OrganizationName: organizationName, PluginName: pluginName,
		}.Build(),
		Version: pluginVersion,
	}.Build())
	if err != nil {
		return Definition{}, fmt.Errorf("GetPluginDefinition RPC: %w", err)
	}

	return Definition{Manifest: resp.GetManifest(), Hash: resp.GetDefinitionHash()}, nil
}
