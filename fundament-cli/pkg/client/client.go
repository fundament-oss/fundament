// Package client provides an authenticated API client for the Fundament CLI.
package client

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"connectrpc.com/connect"

	authnv1 "github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1"
	"github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1/authnv1connect"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

// Client provides authenticated access to Fundament APIs.
type Client struct {
	apiKey      string
	apiEndpoint string
	authnURL    string

	mu     sync.Mutex
	jwt    string
	expiry time.Time

	httpClient  *http.Client
	tokenClient authnv1connect.TokenServiceClient
}

// New creates a new API client.
func New(apiKey, apiEndpoint, authnURL string) *Client {
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	c := &Client{
		apiKey:      apiKey,
		apiEndpoint: apiEndpoint,
		authnURL:    authnURL,
		httpClient:  httpClient,
	}

	// Create the token client without auth (we'll add the API key manually)
	c.tokenClient = authnv1connect.NewTokenServiceClient(httpClient, authnURL)

	return c
}

// ensureToken ensures we have a valid JWT, exchanging the API key if necessary.
func (c *Client) ensureToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Return cached token if still valid (with 30 second buffer)
	if c.jwt != "" && time.Now().Add(30*time.Second).Before(c.expiry) {
		return c.jwt, nil
	}

	// Exchange API key for JWT
	req := connect.NewRequest(&authnv1.ExchangeTokenRequest{})
	req.Header().Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.tokenClient.ExchangeToken(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to exchange API key for token: %w", err)
	}

	c.jwt = resp.Msg.AccessToken
	c.expiry = time.Now().Add(time.Duration(resp.Msg.ExpiresIn) * time.Second)

	return c.jwt, nil
}

// authInterceptor returns a connect interceptor that adds authentication.
func (c *Client) authInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			token, err := c.ensureToken(ctx)
			if err != nil {
				return nil, err
			}
			req.Header().Set("Authorization", "Bearer "+token)
			return next(ctx, req)
		}
	}
}

// clusters returns the cluster service client.
func (c *Client) clusters() organizationv1connect.ClusterServiceClient {
	return organizationv1connect.NewClusterServiceClient(
		c.httpClient,
		c.apiEndpoint,
		connect.WithInterceptors(c.authInterceptor()),
	)
}

// projects returns the project service client.
func (c *Client) projects() organizationv1connect.ProjectServiceClient {
	return organizationv1connect.NewProjectServiceClient(
		c.httpClient,
		c.apiEndpoint,
		connect.WithInterceptors(c.authInterceptor()),
	)
}

// apiKeys returns the API key service client.
func (c *Client) apiKeys() organizationv1connect.APIKeyServiceClient {
	return organizationv1connect.NewAPIKeyServiceClient(
		c.httpClient,
		c.apiEndpoint,
		connect.WithInterceptors(c.authInterceptor()),
	)
}

// authn returns the authn service client (for user info).
func (c *Client) authn() authnv1connect.AuthnServiceClient {
	return authnv1connect.NewAuthnServiceClient(
		c.httpClient,
		c.authnURL,
		connect.WithInterceptors(c.authInterceptor()),
	)
}
