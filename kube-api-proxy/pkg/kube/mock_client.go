package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// MockClient returns hardcoded Kubernetes API responses for development and testing.
// It implements http.Handler so it can be used in place of MultiClusterProxy.
type MockClient struct {
	mu               sync.Mutex
	installByCluster map[string][]map[string]any
	fscByCluster     map[string][]map[string]any
	seq              int

	// PluginTemplatesDir is the on-disk root from which `/proxy/console/<file>`
	// requests are served in mock mode. Layout: <dir>/<pluginName>/console/<file>.
	// Empty disables the console asset handler (returns 404 for those paths).
	PluginTemplatesDir string

	// ConsoleAssets is the cross-origin policy stamped on console asset responses
	// (see plugin_console_assets.go).
	ConsoleAssets ConsoleAssetPolicy
}

const crdBasePath = "/apis/apiextensions.k8s.io/v1/customresourcedefinitions"
const pluginInstallationsPath = "/apis/plugins.fundament.io/v1/plugininstallations"

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
		m.installByCluster[clusterID] = []map[string]any{}
		return m.installByCluster[clusterID]
	}
	return items
}

func (m *MockClient) fscItemsForCluster(clusterID string) []map[string]any {
	if m.fscByCluster == nil {
		m.fscByCluster = map[string][]map[string]any{}
	}
	return m.fscByCluster[clusterID]
}

// fscInstallationListJSON merges the static FSCInstallation fixtures with any
// installations created in-memory this session for the given cluster, so the
// list and detail views reflect a create round-trip in mock mode. With nothing
// created it returns the static fixture verbatim.
func (m *MockClient) fscInstallationListJSON(clusterID string) string {
	m.mu.Lock()
	created := append([]map[string]any(nil), m.fscItemsForCluster(clusterID)...)
	m.mu.Unlock()
	if len(created) == 0 {
		return mockFSCInstallationListJSON
	}

	var list struct {
		APIVersion string           `json:"apiVersion"`
		Kind       string           `json:"kind"`
		Metadata   map[string]any   `json:"metadata"`
		Items      []map[string]any `json:"items"`
	}
	if err := json.Unmarshal([]byte(mockFSCInstallationListJSON), &list); err != nil {
		return mockFSCInstallationListJSON
	}
	list.Items = append(list.Items, created...)
	b, err := json.Marshal(list)
	if err != nil {
		return mockFSCInstallationListJSON
	}
	return string(b)
}

// createFSCInstallation handles a POST to the namespaced fscinstallations
// collection: it fills the server-set metadata/status the console reads back,
// stores the object in-memory for the cluster, and echoes it as a 201.
func (m *MockClient) createFSCInstallation(clusterID, path string, body io.Reader, r func(string) io.ReadCloser) (int, io.ReadCloser, error) {
	var obj map[string]any
	if err := json.NewDecoder(body).Decode(&obj); err != nil {
		return 400, r(`{"message":"invalid body"}`), nil
	}

	meta, _ := obj["metadata"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
		obj["metadata"] = meta
	}
	if _, ok := meta["namespace"]; !ok {
		if ns := resourceNamespaceFromPath(path); ns != "" {
			meta["namespace"] = ns
		}
	}

	m.mu.Lock()
	m.seq++
	meta["uid"] = fmt.Sprintf("fsci-mock-%d", m.seq)
	meta["creationTimestamp"] = time.Now().UTC().Format(time.RFC3339)
	obj["status"] = map[string]any{"phase": "Pending"}
	m.fscByCluster[clusterID] = append(m.fscItemsForCluster(clusterID), obj)
	m.mu.Unlock()

	b, err := json.Marshal(obj)
	if err != nil {
		return 500, r(`{"message":"mock marshal error"}`), nil
	}
	return 201, r(string(b)), nil
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
	case strings.HasPrefix(path, pluginInstallationsPath+"/") && method == http.MethodGet:
		name := path[len(pluginInstallationsPath)+1:]
		m.mu.Lock()
		items := m.installItemsForCluster(clusterID)
		var found []byte
		for _, item := range items {
			meta, _ := item["metadata"].(map[string]any)
			if meta["name"] == name {
				found, _ = json.Marshal(item)
				break
			}
		}
		m.mu.Unlock()
		if found != nil {
			return 200, r(string(found)), nil
		}
		return 404, r(`{"message":"not found"}`), nil
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
	case isPluginGetDefinition(path, "openfsc"):
		return 200, r(mockOpenfscDefinitionJSON), nil
	case isResourceList(path, "cert-manager.io", "v1", "certificates"):
		return 200, r(mockCertificateListJSON), nil
	case isResourceGet(path, "cert-manager.io", "v1", "certificates"):
		return resourceGetResponse(mockCertificateListJSON, resourceNameFromPath(path), resourceNamespaceFromPath(path), r)
	case isResourceList(path, "cert-manager.io", "v1", "certificaterequests"):
		return 200, r(mockCertificateRequestListJSON), nil
	case isResourceGet(path, "cert-manager.io", "v1", "certificaterequests"):
		return resourceGetResponse(mockCertificateRequestListJSON, resourceNameFromPath(path), resourceNamespaceFromPath(path), r)
	case isResourceList(path, "cert-manager.io", "v1", "clusterissuers"):
		return 200, r(mockClusterIssuerListJSON), nil
	case isResourceGet(path, "cert-manager.io", "v1", "clusterissuers"):
		return resourceGetResponse(mockClusterIssuerListJSON, resourceNameFromPath(path), resourceNamespaceFromPath(path), r)
	case isResourceList(path, "cert-manager.io", "v1", "issuers"):
		return 200, r(mockIssuerListJSON), nil
	case isResourceGet(path, "cert-manager.io", "v1", "issuers"):
		return resourceGetResponse(mockIssuerListJSON, resourceNameFromPath(path), resourceNamespaceFromPath(path), r)
	case isResourceList(path, "postgresql.cnpg.io", "v1", "databases"):
		return 200, r(mockDatabaseListJSON), nil
	case isResourceGet(path, "postgresql.cnpg.io", "v1", "databases"):
		return resourceGetResponse(mockDatabaseListJSON, resourceNameFromPath(path), resourceNamespaceFromPath(path), r)
	case isResourceList(path, "postgresql.cnpg.io", "v1", "backups"):
		return 200, r(mockBackupListJSON), nil
	case isResourceGet(path, "postgresql.cnpg.io", "v1", "backups"):
		return resourceGetResponse(mockBackupListJSON, resourceNameFromPath(path), resourceNamespaceFromPath(path), r)
	case isResourceList(path, "postgresql.cnpg.io", "v1", "subscriptions"):
		return 200, r(mockSubscriptionListJSON), nil
	case isResourceGet(path, "postgresql.cnpg.io", "v1", "subscriptions"):
		return resourceGetResponse(mockSubscriptionListJSON, resourceNameFromPath(path), resourceNamespaceFromPath(path), r)
	case isResourceList(path, "demo.fundament.io", "v1", "demoitems"):
		return 200, r(mockDemoItemListJSON), nil
	case isResourceGet(path, "demo.fundament.io", "v1", "demoitems"):
		return resourceGetResponse(mockDemoItemListJSON, resourceNameFromPath(path), resourceNamespaceFromPath(path), r)
	case isResourceList(path, "openfsc.fundament.io", "v1", "fscinstallations") && method == http.MethodPost:
		return m.createFSCInstallation(clusterID, path, body, r)
	case isResourceList(path, "openfsc.fundament.io", "v1", "fscinstallations"):
		return 200, r(m.fscInstallationListJSON(clusterID)), nil
	case isResourceGet(path, "openfsc.fundament.io", "v1", "fscinstallations"):
		return resourceGetResponse(m.fscInstallationListJSON(clusterID), resourceNameFromPath(path), resourceNamespaceFromPath(path), r)
	default:
		return 200, r(mockEmptyList), nil
	}
}

// ServeHTTP implements http.Handler so MockClient can be used in place of MultiClusterProxy.
func (m *MockClient) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Console asset paths are handled separately so we can stream files from
	// disk and set the right Content-Type per extension. They are matched
	// before falling through to the JSON-only Do() handler.
	if pluginName, asset, ok := pluginConsoleAsset(r.URL.Path); ok {
		m.serveConsoleAsset(w, r, pluginName, asset)
		return
	}
	// Addressed at the console route but with an asset path we refuse to serve
	// (empty, absolute, or containing ".."). Reject it outright rather than let it
	// fall through to the JSON resource handler, which would answer 200.
	if _, _, ok := pluginConsoleRoute(r.URL.Path); ok {
		http.Error(w, `{"message":"invalid asset path"}`, http.StatusBadRequest)
		return
	}

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

// serveConsoleAsset serves a static file from PluginTemplatesDir for paths of
// the form /api/v1/namespaces/plugin-<name>/services/http:plugin-<name>:8080/proxy/console/<asset>.
// In real mode the same path is answered by the plugin pod's embedded console FS.
func (m *MockClient) serveConsoleAsset(w http.ResponseWriter, _ *http.Request, pluginName, asset string) {
	if m.PluginTemplatesDir == "" {
		http.Error(w, `{"message":"plugin templates directory not configured"}`, http.StatusNotFound)
		return
	}

	full := filepath.Join(m.PluginTemplatesDir, pluginName, "console", filepath.FromSlash(asset))
	data, err := os.ReadFile(full) //nolint:gosec // pluginName + asset are extracted from a fixed pattern; pluginConsoleAsset rejects "..".
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"message":"failed to read asset"}`, http.StatusInternalServerError)
		return
	}

	contentType := mime.TypeByExtension(filepath.Ext(asset))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	m.ConsoleAssets.SetHeaders(w.Header())
	// Mock mode serves edits live from disk; disable caching so iframe reloads
	// always pick up the latest template without manual cache-busting.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
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

// isResourceGet reports whether path is a Kubernetes single-object get for the
// given group/version/plural. Matches both cluster-scoped
// (/apis/{g}/{v}/{plural}/{name}) and namespaced
// (/apis/{g}/{v}/namespaces/{ns}/{plural}/{name}).
func isResourceGet(path, group, version, plural string) bool { //nolint:unparam // version mirrors isResourceList for symmetry.
	prefix := "/apis/" + group + "/" + version + "/"
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	rest := path[len(prefix):]
	parts := strings.Split(rest, "/")
	// Cluster-scoped: {plural}/{name} (2 parts).
	if len(parts) == 2 && parts[0] == plural && parts[1] != "" {
		return true
	}
	// Namespaced: namespaces/{ns}/{plural}/{name} (4 parts).
	if len(parts) == 4 && parts[0] == "namespaces" && parts[2] == plural && parts[3] != "" {
		return true
	}
	return false
}

// resourceNameFromPath returns the trailing name segment from a single-object
// resource path. Returns "" if the path is not in a recognized shape.
func resourceNameFromPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

// resourceNamespaceFromPath returns the namespace segment from a namespaced
// path (.../namespaces/{ns}/...). Returns "" for cluster-scoped paths.
func resourceNamespaceFromPath(path string) string {
	parts := strings.Split(path, "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "namespaces" {
			return parts[i+1]
		}
	}
	return ""
}

// resourceGetResponse extracts the single item matching name (and namespace, when
// the path is namespaced) from a list JSON document and returns it as the body of
// a 200 response. Returns 404 if no such item exists. Matching on namespace
// disambiguates objects that share a name across namespaces. The list document
// must have shape {"items": [...]}.
func resourceGetResponse(listJSON, name, namespace string, r func(string) io.ReadCloser) (int, io.ReadCloser, error) {
	var data struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal([]byte(listJSON), &data); err != nil {
		return 500, r(`{"message":"mock list parse error"}`), nil
	}
	for _, item := range data.Items {
		meta, _ := item["metadata"].(map[string]any)
		if meta == nil || meta["name"] != name {
			continue
		}
		if namespace != "" && meta["namespace"] != namespace {
			continue
		}
		b, err := json.Marshal(item)
		if err != nil {
			return 500, r(`{"message":"mock item marshal error"}`), nil
		}
		return 200, r(string(b)), nil
	}
	return 404, r(`{"message":"not found"}`), nil
}

// IsPluginConsoleAssetPath reports whether path is a request for a plugin
// console asset. Such requests serve static plugin UI files (HTML/JS/CSS)
// and are treated as public — the sandboxed iframe that loads them cannot
// send credentials, and the assets themselves expose no user-specific data.
func IsPluginConsoleAssetPath(path string) bool {
	_, _, ok := pluginConsoleAsset(path)
	return ok
}

// pluginConsoleAsset matches `/api/v1/namespaces/plugin-<name>/services/http:plugin-<name>:8080/proxy/console/<asset>`
// and returns the plugin name and the trailing asset path, but only for an asset
// path that is safe to serve.
//
// The safety check lives here rather than in each caller because matching this
// pattern both skips the proxy's auth check and stamps a public CORS policy on the
// response (see IsPluginConsoleAssetPath and ConsoleAssetPolicy). A traversal that
// escaped the console directory would otherwise be served unauthenticated *and* be
// readable cross-origin. Rejecting it here means an unsafe path is simply not a
// console asset: real mode falls through to the normal authenticated proxy path,
// and mock mode rejects it (see pluginConsoleRoute's use in ServeHTTP).
//
// Falling through is safe because a traversal never reaches the handler over a real
// connection in the first place: http.ServeMux cleans the request path and redirects
// when cleaning changes it, so a ".." segment is resolved before routing. This
// function is the guarantee that such a path cannot masquerade as a *public* asset;
// the mux is the guarantee it cannot reach the upstream apiserver un-normalized.
func pluginConsoleAsset(path string) (pluginName, asset string, ok bool) {
	pluginName, asset, ok = pluginConsoleRoute(path)
	if !ok || !isSafeAssetPath(asset) {
		return "", "", false
	}
	return pluginName, asset, true
}

// pluginConsoleRoute matches the console-asset URL shape without judging whether
// the asset path is safe to serve. Callers that need "is this route addressed at
// the console, even if malformed?" use this; everyone else wants pluginConsoleAsset.
func pluginConsoleRoute(path string) (pluginName, asset string, ok bool) {
	const (
		nsPrefix  = "/api/v1/namespaces/plugin-"
		svcMid    = "/services/http:plugin-"
		svcSuffix = ":8080/proxy/console/"
	)
	if !strings.HasPrefix(path, nsPrefix) {
		return "", "", false
	}
	afterNS := path[len(nsPrefix):]
	slash := strings.Index(afterNS, "/")
	if slash <= 0 {
		return "", "", false
	}
	pluginName = afterNS[:slash]
	rest := afterNS[slash:]

	if !strings.HasPrefix(rest, svcMid) {
		return "", "", false
	}
	rest = rest[len(svcMid):]
	if !strings.HasPrefix(rest, pluginName+svcSuffix) {
		return "", "", false
	}
	asset = rest[len(pluginName)+len(svcSuffix):]
	return pluginName, asset, true
}

// isSafeAssetPath reports whether asset is a non-empty relative slash-separated
// path with no ".." segment. Checked per segment rather than with
// strings.Contains so a legitimate filename like "foo..bar.js" is not rejected.
func isSafeAssetPath(asset string) bool {
	if asset == "" || strings.HasPrefix(asset, "/") {
		return false
	}
	for segment := range strings.SplitSeq(asset, "/") {
		if segment == ".." {
			return false
		}
	}
	return true
}
