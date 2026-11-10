package provider

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/proto/gen/install/v1/installv1connect"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

// FundamentClient wraps the Connect RPC clients for the Fundament API.
type FundamentClient struct {
	ClusterService   organizationv1connect.ClusterServiceClient
	ProjectService   organizationv1connect.ProjectServiceClient
	MemberService    organizationv1connect.MemberServiceClient
	InviteService    organizationv1connect.InviteServiceClient
	NamespaceService organizationv1connect.NamespaceServiceClient
	// InstallService is the marketplace install API; nil without marketplace_install_url.
	InstallService installv1connect.InstallServiceClient

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
		return nil, fmt.Errorf("get token: %w", err)
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
	return transport.RoundTrip(reqClone) //nolint:wrapcheck // decorating RoundTripper passes the inner transport's error through
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
		return nil, fmt.Errorf("get token: %w", err)
	}
	if token != "" {
		clone.Header.Set("Authorization", "Bearer "+token)
	}
	tr := t.transport
	if tr == nil {
		tr = http.DefaultTransport
	}
	return tr.RoundTrip(clone) //nolint:wrapcheck // decorating RoundTripper passes the inner transport's error through
}

// NewFundamentClientWithTokenManager creates a new FundamentClient with API key authentication.
func NewFundamentClientWithTokenManager(endpoint string, tm *TokenManager, organizationID, kubeProxyURL, installURL string) *FundamentClient {
	transport := &AuthTransport{
		TokenSource:    tm,
		OrganizationID: organizationID,
		Transport:      http.DefaultTransport,
	}
	kubeProxyTransport := &tokenTransport{
		tokenSource: tm,
		transport:   http.DefaultTransport,
	}
	return newFundamentClientWithTransport(endpoint, kubeProxyURL, installURL, transport, kubeProxyTransport)
}

func newFundamentClientWithTransport(endpoint, kubeProxyURL, installURL string, transport, kubeProxyTransport http.RoundTripper) *FundamentClient {
	httpClient := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}
	// No hard timeout on the kube-proxy client — callers set a context deadline instead.
	kubeProxyClient := &http.Client{
		Transport: kubeProxyTransport,
	}

	client := &FundamentClient{
		ClusterService:      organizationv1connect.NewClusterServiceClient(httpClient, endpoint),
		ProjectService:      organizationv1connect.NewProjectServiceClient(httpClient, endpoint),
		MemberService:       organizationv1connect.NewMemberServiceClient(httpClient, endpoint),
		InviteService:       organizationv1connect.NewInviteServiceClient(httpClient, endpoint),
		NamespaceService:    organizationv1connect.NewNamespaceServiceClient(httpClient, endpoint),
		KubeProxyURL:        kubeProxyURL,
		KubeProxyHTTPClient: kubeProxyClient,
	}
	if installURL != "" {
		client.InstallService = installv1connect.NewInstallServiceClient(httpClient, installURL)
	}
	return client
}
