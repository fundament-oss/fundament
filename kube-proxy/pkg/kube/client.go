package kube

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client abstracts access to a Kubernetes API server.
type Client interface {
	Do(ctx context.Context, method, path string, body io.Reader) (statusCode int, responseBody io.ReadCloser, err error)
}

// RealClient connects to a real Kubernetes API server using a kubeconfig.
// The HTTP client and host URL are initialized lazily on the first request.
// Auth is handled by the transport created via rest.HTTPClientFor, which supports
// bearer tokens, client certificates, and basic auth from the kubeconfig.
// Exec-based credential plugins (e.g. aws-iam-authenticator) are not supported.
type RealClient struct {
	KubeconfigPath string

	once       sync.Once
	httpClient *http.Client
	host       string
	initErr    error
}

func (r *RealClient) init() {
	cfg, err := clientcmd.BuildConfigFromFlags("", r.KubeconfigPath)
	if err != nil {
		r.initErr = fmt.Errorf("load kubeconfig: %w", err)
		return
	}
	httpClient, err := rest.HTTPClientFor(cfg)
	if err != nil {
		r.initErr = fmt.Errorf("build http client: %w", err)
		return
	}
	r.httpClient = httpClient
	r.host = strings.TrimRight(cfg.Host, "/")
}

func (r *RealClient) Do(ctx context.Context, method, path string, body io.Reader) (int, io.ReadCloser, error) {
	r.once.Do(r.init)
	if r.initErr != nil {
		return 0, nil, r.initErr
	}

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
