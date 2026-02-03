package client

import (
	"context"

	"connectrpc.com/connect"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// ListAPIKeys lists all API keys.
func (c *Client) ListAPIKeys(ctx context.Context) ([]*organizationv1.APIKey, error) {
	resp, err := c.apiKeys().ListAPIKeys(ctx, connect.NewRequest(&organizationv1.ListAPIKeysRequest{}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.ApiKeys, nil
}

// CreateAPIKey creates a new API key.
func (c *Client) CreateAPIKey(ctx context.Context, name string, expiresInDays *int64) (*organizationv1.CreateAPIKeyResponse, error) {
	req := &organizationv1.CreateAPIKeyRequest{
		Name: name,
	}
	if expiresInDays != nil {
		req.ExpiresInDays = expiresInDays
	}
	resp, err := c.apiKeys().CreateAPIKey(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

// RevokeAPIKey revokes an API key.
func (c *Client) RevokeAPIKey(ctx context.Context, apiKeyID string) error {
	_, err := c.apiKeys().RevokeAPIKey(ctx, connect.NewRequest(&organizationv1.RevokeAPIKeyRequest{
		ApiKeyId: apiKeyID,
	}))
	return err
}
