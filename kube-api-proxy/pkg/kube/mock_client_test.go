package kube

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

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
			if (!got && !gotCluster) && tc.want {
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
	defer func() { _ = body.Close() }()
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
	_ = body.Close()
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
		_ = body.Close()
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
	defer func() { _ = body.Close() }()
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

func TestMockCertificateRequestList(t *testing.T) {
	mc := &MockClient{}
	status, body, err := mc.Do(context.Background(), http.MethodGet, "/apis/cert-manager.io/v1/certificaterequests", nil)
	if err != nil {
		t.Fatal(err)
	}
	if status != 200 {
		t.Fatalf("status = %d", status)
	}
	defer func() { _ = body.Close() }()
	b, _ := io.ReadAll(body)
	if !strings.Contains(string(b), `"kind": "CertificateRequestList"`) {
		t.Errorf("body did not contain CertificateRequestList: %s", string(b))
	}
}
