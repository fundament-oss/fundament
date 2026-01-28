package provider

import (
	"net/http"
	"time"

	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

// FundamentClient wraps the Connect RPC clients for the Fundament API.
type FundamentClient struct {
	ClusterService organizationv1connect.ClusterServiceClient
	ProjectService organizationv1connect.ProjectServiceClient
}

// AuthTransport is an http.RoundTripper that adds a Bearer token to requests.
type AuthTransport struct {
	Token     string
	Transport http.RoundTripper
}

// RoundTrip implements http.RoundTripper.
func (t *AuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	reqClone := req.Clone(req.Context())
	if t.Token != "" {
		reqClone.Header.Set("Authorization", "Bearer "+t.Token)
	}

	transport := t.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	return transport.RoundTrip(reqClone)
}

// NewFundamentClient creates a new FundamentClient with the given endpoint and token.
func NewFundamentClient(endpoint, token string) *FundamentClient {
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &AuthTransport{
			Token:     token,
			Transport: http.DefaultTransport,
		},
	}

	return &FundamentClient{
		ClusterService: organizationv1connect.NewClusterServiceClient(httpClient, endpoint),
		ProjectService: organizationv1connect.NewProjectServiceClient(httpClient, endpoint),
	}
}
