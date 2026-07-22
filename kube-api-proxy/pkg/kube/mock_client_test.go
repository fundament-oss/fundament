package kube

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPluginConsoleAsset(t *testing.T) {
	cases := []struct {
		path      string
		wantName  string
		wantAsset string
		wantOk    bool
	}{
		{
			path:      "/api/v1/namespaces/plugin-cert-manager/services/http:plugin-cert-manager:8080/proxy/console/certificates-list.html",
			wantName:  "cert-manager",
			wantAsset: "certificates-list.html",
			wantOk:    true,
		},
		{
			path:      "/api/v1/namespaces/plugin-demo/services/http:plugin-demo:8080/proxy/console/_shared.js",
			wantName:  "demo",
			wantAsset: "_shared.js",
			wantOk:    true,
		},
		{
			// Exact GetDefinition path is not a console asset.
			path:   "/api/v1/namespaces/plugin-cert-manager/services/http:plugin-cert-manager:8080/proxy/pluginmetadata.v1.PluginMetadataService/GetDefinition",
			wantOk: false,
		},
		{
			// Unrelated kube path.
			path:   "/apis/cert-manager.io/v1/certificates",
			wantOk: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			name, asset, ok := pluginConsoleAsset(tc.path)
			require.Equal(t, tc.wantOk, ok)
			require.Equal(t, tc.wantName, name)
			require.Equal(t, tc.wantAsset, asset)
		})
	}
}

func TestIsResourceGet(t *testing.T) {
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"namespaced get", "/apis/cert-manager.io/v1/namespaces/default/certificates/web-tls", true},
		{"cluster-scoped get", "/apis/cert-manager.io/v1/clusterissuers/letsencrypt-prod", true},
		{"namespaced list", "/apis/cert-manager.io/v1/namespaces/default/certificates", false},
		{"cluster-scoped list", "/apis/cert-manager.io/v1/clusterissuers", false},
		{"different group", "/apis/postgresql.cnpg.io/v1/databases/foo", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isResourceGet(tc.path, "cert-manager.io", "v1", "certificates")
			gotCluster := isResourceGet(tc.path, "cert-manager.io", "v1", "clusterissuers")
			if !(got || gotCluster) && tc.want {
				t.Fatalf("expected match, got none for %q", tc.path)
			}
			if (got || gotCluster) && !tc.want {
				t.Fatalf("unexpected match for %q", tc.path)
			}
		})
	}
}

func TestResourceGetResponse(t *testing.T) {
	r := func(s string) io.ReadCloser { return io.NopCloser(strings.NewReader(s)) }

	status, body, err := resourceGetResponse(mockCertificateListJSON, "web-tls-cert", "", r)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if status != 200 {
		t.Fatalf("status = %d", status)
	}
	defer body.Close()
	b, _ := io.ReadAll(body)
	var item map[string]any
	if err := json.Unmarshal(b, &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	meta, _ := item["metadata"].(map[string]any)
	if meta["name"] != "web-tls-cert" {
		t.Fatalf("wrong name: %v", meta["name"])
	}

	status, body, _ = resourceGetResponse(mockCertificateListJSON, "missing", "", r)
	if status != 404 {
		t.Fatalf("expected 404, got %d", status)
	}
	body.Close()
}

func TestResourceGetResponseNamespaceDisambiguation(t *testing.T) {
	r := func(s string) io.ReadCloser { return io.NopCloser(strings.NewReader(s)) }

	// mockDatabaseListJSON has two "app-db" objects, in "default" and "analytics".
	// A namespaced get must return the object from the requested namespace.
	for _, ns := range []string{"default", "analytics"} {
		status, body, err := resourceGetResponse(mockDatabaseListJSON, "app-db", ns, r)
		if err != nil {
			t.Fatalf("ns %q: err: %v", ns, err)
		}
		if status != 200 {
			t.Fatalf("ns %q: status = %d", ns, status)
		}
		b, _ := io.ReadAll(body)
		body.Close()
		var item map[string]any
		if err := json.Unmarshal(b, &item); err != nil {
			t.Fatalf("ns %q: unmarshal: %v", ns, err)
		}
		meta, _ := item["metadata"].(map[string]any)
		if meta["name"] != "app-db" || meta["namespace"] != ns {
			t.Fatalf("ns %q: got name=%v namespace=%v", ns, meta["name"], meta["namespace"])
		}
	}
}

func TestMockClientDoResourceGet(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	// Namespaced single-object get must return the object (with spec) from the
	// requested namespace — the path the generated detail view now uses.
	status, body, err := m.Do(ctx, http.MethodGet,
		"/apis/postgresql.cnpg.io/v1/namespaces/analytics/databases/app-db", nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if status != 200 {
		t.Fatalf("status = %d", status)
	}
	defer body.Close()
	b, _ := io.ReadAll(body)
	var obj map[string]any
	if err := json.Unmarshal(b, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["kind"] != "Database" {
		t.Fatalf("expected a single Database object, got kind=%v", obj["kind"])
	}
	meta, _ := obj["metadata"].(map[string]any)
	if meta["namespace"] != "analytics" {
		t.Fatalf("wrong namespace: %v", meta["namespace"])
	}
	if _, ok := obj["spec"].(map[string]any); !ok {
		t.Fatalf("returned object has no spec: %s", b)
	}
}

func TestCertManagerDefinitionShape(t *testing.T) {
	var def struct {
		CustomComponents map[string]struct {
			List   string `json:"list"`
			Detail string `json:"detail"`
		} `json:"customComponents"`
		AllowedResources []struct {
			Group    string   `json:"group"`
			Version  string   `json:"version"`
			Resource string   `json:"resource"`
			Verbs    []string `json:"verbs"`
		} `json:"allowedResources"`
		CRDs []string `json:"crds"`
	}
	if err := json.Unmarshal([]byte(mockCertManagerDefinitionJSON), &def); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, kind := range []string{"Certificate", "CertificateRequest", "Issuer", "ClusterIssuer"} {
		if def.CustomComponents[kind].List == "" || def.CustomComponents[kind].Detail == "" {
			t.Errorf("missing customComponents for %s", kind)
		}
	}
	if len(def.AllowedResources) != 4 {
		t.Errorf("expected 4 allowedResources, got %d", len(def.AllowedResources))
	}
	if len(def.CRDs) != 4 {
		t.Errorf("expected 4 crds, got %d", len(def.CRDs))
	}
}

func TestServeConsoleAsset(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "cert-manager", "console")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "list.html"), []byte("<html><body>test</body></html>"), 0o600); err != nil {
		t.Fatal(err)
	}

	mc := &MockClient{PluginTemplatesDir: dir}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/plugin-cert-manager/services/http:plugin-cert-manager:8080/proxy/console/list.html", nil)
	w := httptest.NewRecorder()
	mc.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body = %s", w.Body.String())
	require.True(t, strings.HasPrefix(w.Header().Get("Content-Type"), "text/html"), "Content-Type = %q", w.Header().Get("Content-Type"))
	require.Contains(t, w.Body.String(), "<body>test</body>")

	// Path traversal protection.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/plugin-cert-manager/services/http:plugin-cert-manager:8080/proxy/console/../../etc/passwd", nil)
	w2 := httptest.NewRecorder()
	mc.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusBadRequest, w2.Code, "expected 400 for traversal")

	// Missing file → 404.
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/plugin-cert-manager/services/http:plugin-cert-manager:8080/proxy/console/nope.html", nil)
	w3 := httptest.NewRecorder()
	mc.ServeHTTP(w3, req3)
	require.Equal(t, http.StatusNotFound, w3.Code, "expected 404 for missing file")
}

func TestMockFSCInstallations(t *testing.T) {
	mc := &MockClient{}

	// List.
	status, body, err := mc.Do(context.Background(), http.MethodGet, "/apis/openfsc.fundament.io/v1/fscinstallations", nil)
	if err != nil {
		t.Fatal(err)
	}
	if status != 200 {
		t.Fatalf("list status = %d", status)
	}
	b, _ := io.ReadAll(body)
	body.Close()
	if !strings.Contains(string(b), `"kind": "FSCInstallationList"`) {
		t.Errorf("body did not contain FSCInstallationList: %s", string(b))
	}

	// Namespaced get of a known item.
	status, body, err = mc.Do(context.Background(), http.MethodGet, "/apis/openfsc.fundament.io/v1/namespaces/fsc-demo/fscinstallations/demo", nil)
	if err != nil {
		t.Fatal(err)
	}
	if status != 200 {
		t.Fatalf("get status = %d", status)
	}
	b, _ = io.ReadAll(body)
	body.Close()
	var item map[string]any
	if err := json.Unmarshal(b, &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	meta, _ := item["metadata"].(map[string]any)
	if meta["name"] != "demo" {
		t.Fatalf("wrong name: %v", meta["name"])
	}

	// GetDefinition for the openfsc plugin.
	status, body, err = mc.Do(context.Background(), http.MethodGet, "/api/v1/namespaces/plugin-openfsc/services/http:plugin-openfsc:8080/proxy/pluginmetadata.v1.PluginMetadataService/GetDefinition", nil)
	if err != nil {
		t.Fatal(err)
	}
	if status != 200 {
		t.Fatalf("definition status = %d", status)
	}
	b, _ = io.ReadAll(body)
	body.Close()
	if !strings.Contains(string(b), `"name": "openfsc"`) {
		t.Errorf("definition body did not contain openfsc: %s", string(b))
	}

	// CRD is registered and resolvable by name.
	if _, ok := mockCRDForName("fscinstallations.openfsc.fundament.io"); !ok {
		t.Error("fscinstallations CRD not registered in mockCRDForName")
	}
}

func TestMockFSCInstallationCreate(t *testing.T) {
	mc := &MockClient{}
	ctx := context.WithValue(context.Background(), ClusterIDContextKey{}, "c1")

	createJSON := `{"apiVersion":"openfsc.fundament.io/v1","kind":"FSCInstallation",` +
		`"metadata":{"name":"new-peer","namespace":"team-a"},` +
		`"spec":{"groupID":"g","peerID":"p","directory":{"mode":"Self"},"postgres":{"storageClass":"local-path"}}}`

	status, body, err := mc.Do(ctx, http.MethodPost,
		"/apis/openfsc.fundament.io/v1/namespaces/team-a/fscinstallations", strings.NewReader(createJSON))
	if err != nil {
		t.Fatal(err)
	}
	if status != 201 {
		t.Fatalf("create status = %d", status)
	}
	b, _ := io.ReadAll(body)
	body.Close()
	var created map[string]any
	if err := json.Unmarshal(b, &created); err != nil {
		t.Fatalf("unmarshal created: %v", err)
	}
	meta, _ := created["metadata"].(map[string]any)
	if meta["name"] != "new-peer" || meta["namespace"] != "team-a" {
		t.Fatalf("unexpected created metadata: %v", meta)
	}
	if meta["uid"] == nil || meta["creationTimestamp"] == nil {
		t.Errorf("server-set fields missing: %v", meta)
	}

	// The new installation appears in the list and via a namespaced get.
	_, body, _ = mc.Do(ctx, http.MethodGet, "/apis/openfsc.fundament.io/v1/fscinstallations", nil)
	b, _ = io.ReadAll(body)
	body.Close()
	if !strings.Contains(string(b), "new-peer") {
		t.Errorf("created item not in list: %s", string(b))
	}

	status, body, _ = mc.Do(ctx, http.MethodGet,
		"/apis/openfsc.fundament.io/v1/namespaces/team-a/fscinstallations/new-peer", nil)
	if status != 200 {
		t.Fatalf("get status = %d", status)
	}
	body.Close()

	// Creations are scoped per cluster.
	otherCtx := context.WithValue(context.Background(), ClusterIDContextKey{}, "c2")
	_, body, _ = mc.Do(otherCtx, http.MethodGet, "/apis/openfsc.fundament.io/v1/fscinstallations", nil)
	b, _ = io.ReadAll(body)
	body.Close()
	if strings.Contains(string(b), "new-peer") {
		t.Errorf("cluster isolation broken: %s", string(b))
	}
}

func TestMockCertificateRequestList(t *testing.T) {
	mc := &MockClient{}
	status, body, err := mc.Do(context.Background(), http.MethodGet, "/apis/cert-manager.io/v1/certificaterequests", nil)
	if err != nil {
		t.Fatal(err)
	}
	if status != 200 {
		t.Fatalf("status = %d", status)
	}
	defer body.Close()
	b, _ := io.ReadAll(body)
	if !strings.Contains(string(b), `"kind": "CertificateRequestList"`) {
		t.Errorf("body did not contain CertificateRequestList: %s", string(b))
	}
}
