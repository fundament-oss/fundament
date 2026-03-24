package kube

import (
	"context"
	"io"
	"net/http"
	"strings"
)

// MockClient returns hardcoded Kubernetes API responses for development and testing.
// It implements both Interface (Do) and http.Handler (ServeHTTP).
type MockClient struct{}

const crdBasePath = "/apis/apiextensions.k8s.io/v1/customresourcedefinitions"

func (m *MockClient) Do(_ context.Context, _, path string, _ io.Reader) (int, io.ReadCloser, error) {
	r := func(s string) io.ReadCloser { return io.NopCloser(strings.NewReader(s)) }

	// Strip query string for matching.
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}

	switch {
	case strings.HasPrefix(path, crdBasePath+"/"):
		name := path[len(crdBasePath)+1:]
		if crd, ok := mockCRDForName(name); ok {
			return 200, r(crd), nil
		}
		return 404, r(`{"message":"not found"}`), nil
	case path == crdBasePath:
		return 200, r(mockCRDListJSON), nil
	case strings.HasPrefix(path, "/apis/cert-manager.io/v1/") && strings.HasSuffix(path, "/certificates"):
		return 200, r(mockCertificateListJSON), nil
	case strings.HasPrefix(path, "/apis/cert-manager.io/v1/") && strings.HasSuffix(path, "/clusterissuers"):
		return 200, r(mockClusterIssuerListJSON), nil
	case strings.HasPrefix(path, "/apis/cert-manager.io/v1/") && strings.HasSuffix(path, "/issuers"):
		return 200, r(mockIssuerListJSON), nil
	case strings.HasPrefix(path, "/apis/postgresql.cnpg.io/v1/") && strings.HasSuffix(path, "/databases"):
		return 200, r(mockDatabaseListJSON), nil
	case strings.HasPrefix(path, "/apis/postgresql.cnpg.io/v1/") && strings.HasSuffix(path, "/backups"):
		return 200, r(mockBackupListJSON), nil
	case strings.HasPrefix(path, "/apis/postgresql.cnpg.io/v1/") && strings.HasSuffix(path, "/subscriptions"):
		return 200, r(mockSubscriptionListJSON), nil
	default:
		return 200, r(mockEmptyList), nil
	}
}

// ServeHTTP implements http.Handler so MockClient can be used in place of MultiClusterProxy.
func (m *MockClient) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/k8s-api")
	if r.URL.RawQuery != "" {
		path = path + "?" + r.URL.RawQuery
	}

	statusCode, body, err := m.Do(r.Context(), r.Method, path, r.Body)
	if err != nil {
		http.Error(w, "failed to contact kubernetes API", http.StatusBadGateway)
		return
	}
	defer body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = io.Copy(w, body)
}

const mockEmptyList = `{"apiVersion":"v1","kind":"List","metadata":{"resourceVersion":""},"items":[]}`
