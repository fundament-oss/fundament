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
			if ok != tc.wantOk || name != tc.wantName || asset != tc.wantAsset {
				t.Fatalf("got (%q, %q, %v); want (%q, %q, %v)", name, asset, ok, tc.wantName, tc.wantAsset, tc.wantOk)
			}
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

	status, body, err := resourceGetResponse(mockCertificateListJSON, "web-tls-cert", r)
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

	status, body, _ = resourceGetResponse(mockCertificateListJSON, "missing", r)
	if status != 404 {
		t.Fatalf("expected 404, got %d", status)
	}
	body.Close()
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

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Errorf("Content-Type = %q", got)
	}
	if !strings.Contains(w.Body.String(), "<body>test</body>") {
		t.Errorf("unexpected body: %s", w.Body.String())
	}

	// Path traversal protection.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/plugin-cert-manager/services/http:plugin-cert-manager:8080/proxy/console/../../etc/passwd", nil)
	w2 := httptest.NewRecorder()
	mc.ServeHTTP(w2, req2)
	if w2.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for traversal, got %d", w2.Code)
	}

	// Missing file → 404.
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/plugin-cert-manager/services/http:plugin-cert-manager:8080/proxy/console/nope.html", nil)
	w3 := httptest.NewRecorder()
	mc.ServeHTTP(w3, req3)
	if w3.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing file, got %d", w3.Code)
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
