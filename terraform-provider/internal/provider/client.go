package provider

import (
	"context"
	"net/http"
	"time"

	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

// FundamentClient wraps the Connect RPC clients for the Fundament API.
type FundamentClient struct {
	ClusterService   organizationv1connect.ClusterServiceClient
	ProjectService   organizationv1connect.ProjectServiceClient
	MemberService    organizationv1connect.MemberServiceClient
	InviteService    organizationv1connect.InviteServiceClient
	NamespaceService organizationv1connect.NamespaceServiceClient

	// KubeProxyURL is the base URL for the kube-api-proxy service.
	KubeProxyURL        string
	KubeProxyHTTPClient *http.Client
}

// TokenSource provides authentication tokens.
type TokenSource interface {
	GetToken(ctx context.Context) (string, error)
}

// AuthTransport is an http.RoundTripper that adds authentication and organization headers to requests.
type AuthTransport struct {
	TokenSource    TokenSource
	OrganizationID string
	Transport      http.RoundTripper
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

	if t.OrganizationID != "" {
		reqClone.Header.Set("Fun-Organization", t.OrganizationID)
	}

	transport := t.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	return transport.RoundTrip(reqClone)
}

// tokenTransport is an http.RoundTripper that adds only a Bearer token — no org header.
// Used for the kube-api-proxy client so that Fundament-specific headers are not leaked.
type tokenTransport struct {
	tokenSource TokenSource
	transport   http.RoundTripper
}

func (t *tokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	token, err := t.tokenSource.GetToken(req.Context())
	if err != nil {
		return nil, err
	}
	if token != "" {
		clone.Header.Set("Authorization", "Bearer "+token)
	}
	tr := t.transport
	if tr == nil {
		tr = http.DefaultTransport
	}
	return tr.RoundTrip(clone)
}

// NewFundamentClientWithTokenManager creates a new FundamentClient with API key authentication.
func NewFundamentClientWithTokenManager(endpoint string, tm *TokenManager, organizationID string, kubeProxyURL string) *FundamentClient {
	transport := &AuthTransport{
		TokenSource:    tm,
		OrganizationID: organizationID,
		Transport:      http.DefaultTransport,
	}
	kubeProxyTransport := &tokenTransport{
		tokenSource: tm,
		transport:   http.DefaultTransport,
	}
	return newFundamentClientWithTransport(endpoint, kubeProxyURL, transport, kubeProxyTransport)
}

func newFundamentClientWithTransport(endpoint string, kubeProxyURL string, transport http.RoundTripper, kubeProxyTransport http.RoundTripper) *FundamentClient {
	httpClient := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}
	// No hard timeout on the kube-proxy client — callers set a context deadline instead.
	kubeProxyClient := &http.Client{
		Transport: kubeProxyTransport,
	}

	return &FundamentClient{
		ClusterService:      organizationv1connect.NewClusterServiceClient(httpClient, endpoint),
		ProjectService:      organizationv1connect.NewProjectServiceClient(httpClient, endpoint),
		MemberService:       organizationv1connect.NewMemberServiceClient(httpClient, endpoint),
		InviteService:       organizationv1connect.NewInviteServiceClient(httpClient, endpoint),
		NamespaceService:    organizationv1connect.NewNamespaceServiceClient(httpClient, endpoint),
		KubeProxyURL:        kubeProxyURL,
		KubeProxyHTTPClient: kubeProxyClient,
	}
}
