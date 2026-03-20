package kube

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client abstracts access to a Kubernetes API server.
type Interface interface {
	Do(ctx context.Context, method, path string, body io.Reader) (statusCode int, responseBody io.ReadCloser, err error)
}

// Client connects to a real Kubernetes API server using a kubeconfig.
// Auth is handled by the transport created via rest.HTTPClientFor, which supports
// bearer tokens, client certificates, and basic auth from the kubeconfig.
// Exec-based credential plugins (e.g. aws-iam-authenticator) are not supported.
type Client struct {
	httpClient *http.Client
	host       *url.URL
}

func New(kubeconfigPath string) (*Client, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
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

func (c *Client) Do(ctx context.Context, method, path string, body io.Reader) (int, io.ReadCloser, error) {
	u := c.host.String() + path
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return 0, nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("kubernetes request: %w", err)
	}

	return resp.StatusCode, resp.Body, nil
}
