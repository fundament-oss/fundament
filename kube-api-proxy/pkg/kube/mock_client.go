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
	case isResourceList(path, "cert-manager.io", "v1", "certificates"):
		return 200, r(mockCertificateListJSON), nil
	case isResourceList(path, "cert-manager.io", "v1", "clusterissuers"):
		return 200, r(mockClusterIssuerListJSON), nil
	case isResourceList(path, "cert-manager.io", "v1", "issuers"):
		return 200, r(mockIssuerListJSON), nil
	case isResourceList(path, "postgresql.cnpg.io", "v1", "databases"):
		return 200, r(mockDatabaseListJSON), nil
	case isResourceList(path, "postgresql.cnpg.io", "v1", "backups"):
		return 200, r(mockBackupListJSON), nil
	case isResourceList(path, "postgresql.cnpg.io", "v1", "subscriptions"):
		return 200, r(mockSubscriptionListJSON), nil
	default:
		return 200, r(mockEmptyList), nil
	}
}

// ServeHTTP implements http.Handler so MockClient can be used in place of MultiClusterProxy.
func (m *MockClient) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if r.URL.RawQuery != "" {
		path = path + "?" + r.URL.RawQuery
	}

	statusCode, body, err := m.Do(r.Context(), r.Method, path, r.Body)
	if err != nil {
		http.Error(w, "failed to contact kubernetes API", http.StatusBadGateway)
		return
	}
	defer func() { _ = body.Close() }()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = io.Copy(w, body)
}

const mockEmptyList = `{"apiVersion":"v1","kind":"List","metadata":{"resourceVersion":""},"items":[]}`

// isResourceList reports whether path is a Kubernetes list request for the given group/version/plural.
// Matches both cluster-scoped (/apis/{g}/{v}/{plural}) and namespaced
// (/apis/{g}/{v}/namespaces/{ns}/{plural}) list paths.
func isResourceList(path, group, version, plural string) bool { //nolint:unparam // version is always v1 today but the param keeps the function general
	prefix := "/apis/" + group + "/" + version + "/"
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	rest := path[len(prefix):]
	// Cluster-scoped: exactly the plural segment.
	if rest == plural {
		return true
	}
	// Namespaced: "namespaces/{ns}/{plural}" — exactly three segments.
	return strings.HasPrefix(rest, "namespaces/") &&
		strings.HasSuffix(rest, "/"+plural) &&
		strings.Count(rest, "/") == 2
}
