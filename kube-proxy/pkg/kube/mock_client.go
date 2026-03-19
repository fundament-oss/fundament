package kube

import (
	"context"
	"io"
	"strings"
)

// MockClient returns hardcoded Kubernetes API responses for development and testing.
type MockClient struct{}

func (m *MockClient) Do(_ context.Context, _, path string, _ io.Reader) (int, io.ReadCloser, error) {
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

const mockEmptyList = `{"apiVersion":"v1","kind":"List","metadata":{"resourceVersion":""},"items":[]}`
