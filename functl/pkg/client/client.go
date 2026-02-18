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

// OrganizationHeader is the header name for selecting the active organization.
const OrganizationHeader = "Fun-Organization"

// Client provides authenticated access to Fundament APIs.
type Client struct {
	apiKey         string
	apiEndpoint    string
	authnURL       string
	organizationID string

	mu     sync.Mutex
	jwt    string
	expiry time.Time

	httpClient  *http.Client
	tokenClient authnv1connect.TokenServiceClient
}

// New creates a new API client.
func New(apiKey, apiEndpoint, authnURL, organizationID string) *Client {
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	c := &Client{
		apiKey:         apiKey,
		apiEndpoint:    apiEndpoint,
		authnURL:       authnURL,
		organizationID: organizationID,
		httpClient:     httpClient,
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
			if c.organizationID != "" {
				req.Header().Set(OrganizationHeader, c.organizationID)
			}
			return next(ctx, req)
		}
	}
}

// Clusters returns the cluster service client.
func (c *Client) Clusters() organizationv1connect.ClusterServiceClient {
	return organizationv1connect.NewClusterServiceClient(
		c.httpClient,
		c.apiEndpoint,
		connect.WithInterceptors(c.authInterceptor()),
	)
}

// Projects returns the project service client.
func (c *Client) Projects() organizationv1connect.ProjectServiceClient {
	return organizationv1connect.NewProjectServiceClient(
		c.httpClient,
		c.apiEndpoint,
		connect.WithInterceptors(c.authInterceptor()),
	)
}

// APIKeys returns the API key service client.
func (c *Client) APIKeys() organizationv1connect.APIKeyServiceClient {
	return organizationv1connect.NewAPIKeyServiceClient(
		c.httpClient,
		c.apiEndpoint,
		connect.WithInterceptors(c.authInterceptor()),
	)
}

// Members returns the member service client.
func (c *Client) Members() organizationv1connect.MemberServiceClient {
	return organizationv1connect.NewMemberServiceClient(
		c.httpClient,
		c.apiEndpoint,
		connect.WithInterceptors(c.authInterceptor()),
	)
}

// Invites returns the invite service client.
func (c *Client) Invites() organizationv1connect.InviteServiceClient {
	return organizationv1connect.NewInviteServiceClient(
		c.httpClient,
		c.apiEndpoint,
		connect.WithInterceptors(c.authInterceptor()),
	)
}

// Authn returns the authn service client (for user info).
func (c *Client) Authn() authnv1connect.AuthnServiceClient {
	return authnv1connect.NewAuthnServiceClient(
		c.httpClient,
		c.authnURL,
		connect.WithInterceptors(c.authInterceptor()),
	)
}
