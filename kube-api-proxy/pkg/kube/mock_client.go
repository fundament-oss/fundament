package kube

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
)

// MockClient returns hardcoded Kubernetes API responses for development and testing.
// It implements http.Handler so it can be used in place of MultiClusterProxy.
type MockClient struct {
	mu               sync.Mutex
	installByCluster map[string][]map[string]any
}

const crdBasePath = "/apis/apiextensions.k8s.io/v1/customresourcedefinitions"
const pluginInstallationsPath = "/apis/plugins.fundament.io/v1/plugininstallations"

var defaultMockInstallItems = []map[string]any{
	{
		"apiVersion": "plugins.fundament.io/v1",
		"kind":       "PluginInstallation",
		"metadata":   map[string]any{"name": "cert-manager", "namespace": "plugin-cert-manager"},
		"spec":       map[string]any{"pluginName": "cert-manager", "image": "mock"},
		"status":     map[string]any{"phase": "Running", "ready": true},
	},
	{
		"apiVersion": "plugins.fundament.io/v1",
		"kind":       "PluginInstallation",
		"metadata":   map[string]any{"name": "CloudNativePG", "namespace": "plugin-CloudNativePG"},
		"spec":       map[string]any{"pluginName": "CloudNativePG", "image": "mock"},
		"status":     map[string]any{"phase": "Running", "ready": true},
	},
}

func clusterIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(ClusterIDContextKey{}).(string)
	return id
}

func (m *MockClient) installItemsForCluster(clusterID string) []map[string]any {
	if m.installByCluster == nil {
		m.installByCluster = map[string][]map[string]any{}
	}
	items, ok := m.installByCluster[clusterID]
	if !ok {
		// Deep-copy the defaults so mutations don't affect other clusters.
		copied := make([]map[string]any, len(defaultMockInstallItems))
		for i, item := range defaultMockInstallItems {
			cp := make(map[string]any, len(item))
			for k, v := range item {
				cp[k] = v
			}
			copied[i] = cp
		}
		m.installByCluster[clusterID] = copied
		return copied
	}
	return items
}

func (m *MockClient) installationsListJSON(clusterID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := m.installItemsForCluster(clusterID)
	list := map[string]any{
		"apiVersion": "plugins.fundament.io/v1",
		"kind":       "PluginInstallationList",
		"metadata":   map[string]any{"resourceVersion": "1"},
		"items":      items,
	}
	b, _ := json.Marshal(list)
	return string(b)
}

func (m *MockClient) Do(ctx context.Context, method, path string, body io.Reader) (int, io.ReadCloser, error) {
	r := func(s string) io.ReadCloser { return io.NopCloser(strings.NewReader(s)) }

	// Strip query string for matching.
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}

	clusterID := clusterIDFromContext(ctx)

	switch {
	case path == pluginInstallationsPath && method == http.MethodPost:
		var obj map[string]any
		if err := json.NewDecoder(body).Decode(&obj); err != nil {
			return 400, r(`{"message":"invalid body"}`), nil
		}
		obj["status"] = map[string]any{"phase": "Running", "ready": true}
		m.mu.Lock()
		items := m.installItemsForCluster(clusterID)
		m.installByCluster[clusterID] = append(items, obj)
		m.mu.Unlock()
		b, _ := json.Marshal(obj)
		return 201, r(string(b)), nil
	case strings.HasPrefix(path, pluginInstallationsPath+"/") && method == http.MethodDelete:
		name := path[len(pluginInstallationsPath)+1:]
		m.mu.Lock()
		items := m.installItemsForCluster(clusterID)
		for i, item := range items {
			meta, _ := item["metadata"].(map[string]any)
			if meta["name"] == name {
				m.installByCluster[clusterID] = append(items[:i], items[i+1:]...)
				break
			}
		}
		m.mu.Unlock()
		return 200, r(`{}`), nil
	case path == pluginInstallationsPath:
		return 200, r(m.installationsListJSON(clusterID)), nil
	case strings.HasPrefix(path, crdBasePath+"/"):
		name := path[len(crdBasePath)+1:]
		if crd, ok := mockCRDForName(name); ok {
			return 200, r(crd), nil
		}
		return 404, r(`{"message":"not found"}`), nil
	case path == crdBasePath:
		return 200, r(mockCRDListJSON), nil
	case isPluginGetDefinition(path, "cert-manager"):
		return 200, r(mockCertManagerDefinitionJSON), nil
	case isPluginGetDefinition(path, "cnpg"), isPluginGetDefinition(path, "CloudNativePG"):
		return 200, r(mockCnpgDefinitionJSON), nil
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
	case isResourceList(path, "demo.fundament.io", "v1", "demoitems"):
		return 200, r(mockDemoItemListJSON), nil
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

// isPluginGetDefinition reports whether path is a Kubernetes service proxy request to
// GetDefinition on the given plugin's metadata service.
func isPluginGetDefinition(path, pluginName string) bool {
	return path == "/api/v1/namespaces/plugin-"+pluginName+"/services/http:plugin-"+pluginName+":8080/proxy/pluginmetadata.v1.PluginMetadataService/GetDefinition"
}

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
