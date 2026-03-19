package kube

import (
	"context"
	"fmt"
	"io"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"net/http"
	"strings"
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
	host       string
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
	return &Client{
		httpClient: httpClient,
		host:       strings.TrimRight(cfg.Host, "/"),
	}, nil
}

func (r *Client) Do(ctx context.Context, method, path string, body io.Reader) (int, io.ReadCloser, error) {
	url := r.host + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return 0, nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("kubernetes request: %w", err)
	}

	return resp.StatusCode, resp.Body, nil
}
