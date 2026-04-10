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

// NewFromBytes creates a Client from in-memory kubeconfig data.
func NewFromBytes(kubeconfigData []byte) (*Client, error) {
	clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeconfigData)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}

	cfg, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build rest config: %w", err)
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
