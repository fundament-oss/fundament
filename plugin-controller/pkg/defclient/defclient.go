package defclient

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

// Definition is the raw, hash-verifiable manifest returned by organization-api.
type Definition struct {
	Manifest []byte
	Hash     string
}

// Client fetches plugin definitions from organization-api.
type Client interface {
	GetDefinition(ctx context.Context, pluginName, pluginVersion string) (Definition, error)
}

type connectClient struct {
	rpc organizationv1connect.PluginServiceClient
}

// New returns a Client that talks to organization-api at baseURL.
func New(baseURL string, httpClient connect.HTTPClient) Client {
	return &connectClient{rpc: organizationv1connect.NewPluginServiceClient(httpClient, baseURL)}
}

func (c *connectClient) GetDefinition(ctx context.Context, pluginName, pluginVersion string) (Definition, error) {
	resp, err := c.rpc.GetPluginDefinition(ctx, connect.NewRequest(organizationv1.GetPluginDefinitionRequest_builder{
		PluginName: pluginName, PluginVersion: pluginVersion,
	}.Build()))
	if err != nil {
		return Definition{}, fmt.Errorf("GetPluginDefinition RPC: %w", err)
	}
	return Definition{Manifest: resp.Msg.GetManifest(), Hash: resp.Msg.GetHash()}, nil
}
