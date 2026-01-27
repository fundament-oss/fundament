package provider

import (
	"context"
	"net/http"
	"time"

	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

// FundamentClient wraps the Connect RPC clients for the Fundament API.
type FundamentClient struct {
	ClusterService organizationv1connect.ClusterServiceClient
}

// TokenSource provides authentication tokens.
type TokenSource interface {
	GetToken(ctx context.Context) (string, error)
}

// AuthTransport is an http.RoundTripper that adds a Bearer token to requests.
type AuthTransport struct {
	TokenSource TokenSource
	Transport   http.RoundTripper
}

// RoundTrip implements http.RoundTripper.
func (t *AuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	reqClone := req.Clone(req.Context())

	token, err := t.TokenSource.GetToken(req.Context())
	if err != nil {
		return nil, err
	}

	if token != "" {
		reqClone.Header.Set("Authorization", "Bearer "+token)
	}

	transport := t.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	return transport.RoundTrip(reqClone)
}

// NewFundamentClient creates a new FundamentClient with static token authentication.
func NewFundamentClient(endpoint, token string) *FundamentClient {
	return newFundamentClientWithTransport(endpoint, &AuthTransport{
		TokenSource: StaticTokenSource(token),
		Transport:   http.DefaultTransport,
	})
}

// NewFundamentClientWithTokenManager creates a new FundamentClient with API key authentication.
func NewFundamentClientWithTokenManager(endpoint string, tm *TokenManager) *FundamentClient {
	return newFundamentClientWithTransport(endpoint, &AuthTransport{
		TokenSource: tm,
		Transport:   http.DefaultTransport,
	})
}

func newFundamentClientWithTransport(endpoint string, transport http.RoundTripper) *FundamentClient {
	httpClient := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	return &FundamentClient{
		ClusterService: organizationv1connect.NewClusterServiceClient(httpClient, endpoint),
	}
}
