package defclient

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	catalogv1 "github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/proto/gen/catalog/v1"
	"github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/proto/gen/install/v1/installv1connect"
	"github.com/fundament-oss/fundament/plugin-controller/pkg/identity"
)

// Definition is the raw, hash-verifiable manifest returned by the install surface.
type Definition struct {
	Manifest []byte
	Hash     string
}

// Client fetches plugin definitions from install.v1 (served by
// marketplace-catalog-api) as this cluster's workload.
type Client interface {
	GetDefinition(ctx context.Context, organizationName, pluginName, pluginVersion string) (Definition, error)
}

type connectClient struct {
	rpc installv1connect.InstallServiceClient
}

// New returns a Client that talks to install.v1 at baseURL, authenticating
// every call with a WorkloadToken from source (FUN-22).
func New(baseURL string, httpClient connect.HTTPClient, source identity.TokenSource) Client {
	return &connectClient{rpc: installv1connect.NewInstallServiceClient(httpClient, baseURL,
		connect.WithInterceptors(identity.BearerInterceptor(source)))}
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
