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

// KubeClient abstracts access to a Kubernetes API server.
type KubeClient interface {
	Do(ctx context.Context, method, path string, body io.Reader) (statusCode int, responseBody io.ReadCloser, err error)
}

// MockKubeClient returns hardcoded Kubernetes API responses for development and testing.
type MockKubeClient struct{}

func (m *MockKubeClient) Do(_ context.Context, _, path string, _ io.Reader) (int, io.ReadCloser, error) {
	r := func(s string) io.ReadCloser { return io.NopCloser(strings.NewReader(s)) }
	switch {
	case strings.Contains(path, "/customresourcedefinitions/"):
		name := path[strings.LastIndex(path, "/")+1:]
		if crd := mockCRDForName(name); crd != "" {
			return 200, r(crd), nil
		}
		return 404, r(`{"message":"not found"}`), nil
	case strings.Contains(path, "customresourcedefinitions"):
		return 200, r(mockCRDListJSON), nil
	case strings.HasSuffix(path, "/certificates"):
		return 200, r(mockCertificateListJSON), nil
	case strings.HasSuffix(path, "/clusterissuers"):
		return 200, r(mockClusterIssuerListJSON), nil
	case strings.HasSuffix(path, "/issuers"):
		return 200, r(mockIssuerListJSON), nil
	case strings.HasSuffix(path, "/databases"):
		return 200, r(mockDatabaseListJSON), nil
	case strings.HasSuffix(path, "/backups"):
		return 200, r(mockBackupListJSON), nil
	case strings.HasSuffix(path, "/subscriptions"):
		return 200, r(mockSubscriptionListJSON), nil
	default:
		return 200, r(mockEmptyList), nil
	}
}

// RealKubeClient connects to a real Kubernetes API server using a kubeconfig.
// The HTTP client and host URL are initialized lazily on the first request.
type RealKubeClient struct {
	KubeconfigPath string

	once       sync.Once
	httpClient *http.Client
	host       string
	initErr    error
}

func (r *RealKubeClient) init() {
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

func (r *RealKubeClient) Do(ctx context.Context, method, path string, body io.Reader) (int, io.ReadCloser, error) {
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

const mockEmptyList = `{"apiVersion":"v1","kind":"List","metadata":{"resourceVersion":""},"items":[]}`
