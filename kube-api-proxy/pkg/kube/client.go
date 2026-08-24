package kube

import (
	"fmt"
	"net/http"
	"net/url"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client connects to a real Kubernetes API server using a kubeconfig.
// Auth is handled by the transport created via rest.HTTPClientFor, which supports
// bearer tokens, client certificates, and basic auth from the kubeconfig.
// Exec-based credential plugins (e.g. aws-iam-authenticator) are not supported.
type Client struct {
	httpClient *http.Client
	host       *url.URL
}

// NewFromBytes creates a Client whose transport uses the kubeconfig's own
// credentials (bearer token, client cert, basic auth).
func NewFromBytes(kubeconfigData []byte) (*Client, error) {
	return newFromBytes(kubeconfigData, false)
}

// NewAnonymousFromBytes creates a Client whose transport carries no client
// credentials, keeping only the server TLS/CA settings. Use it when the caller
// supplies the identity per request (e.g. a bearer token injected by a reverse
// proxy), so a client certificate in the transport can't override it.
func NewAnonymousFromBytes(kubeconfigData []byte) (*Client, error) {
	return newFromBytes(kubeconfigData, true)
}

func newFromBytes(kubeconfigData []byte, anonymous bool) (*Client, error) {
	clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeconfigData)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}

	cfg, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build rest config: %w", err)
	}

	if anonymous {
		cfg = rest.AnonymousClientConfig(cfg)
	}

	httpClient, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("build http client: %w", err)
	}

	host, err := url.Parse(cfg.Host)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig host: %w", err)
	}
	host.Path = ""
	host.RawQuery = ""

	return &Client{
		httpClient: httpClient,
		host:       host,
	}, nil
}

// Host returns the parsed base URL of the Kubernetes API server.
func (c *Client) Host() *url.URL {
	return c.host
}

// Transport returns the http.RoundTripper configured for the Kubernetes API server.
func (c *Client) Transport() http.RoundTripper {
	return c.httpClient.Transport
}
