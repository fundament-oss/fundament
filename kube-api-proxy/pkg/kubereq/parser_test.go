package kubereq

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	cases := []struct {
		name, method, path, query string
		want                      Attributes
	}{
		{"core list", "GET", "/api/v1/pods", "",
			Attributes{APIGroup: "", Resource: "pods", Verb: "list"}},
		{"core get", "GET", "/api/v1/pods/web-1", "",
			Attributes{Resource: "pods", Name: "web-1", Verb: "get"}},
		{"core ns list", "GET", "/api/v1/namespaces/default/pods", "",
			Attributes{Namespace: "default", Resource: "pods", Verb: "list"}},
		{"namespaces list", "GET", "/api/v1/namespaces", "",
			Attributes{Resource: "namespaces", Verb: "list"}},
		{"namespace get", "GET", "/api/v1/namespaces/default", "",
			Attributes{Resource: "namespaces", Name: "default", Verb: "get"}},
		{"namespace delete", "DELETE", "/api/v1/namespaces/default", "",
			Attributes{Resource: "namespaces", Name: "default", Verb: "delete"}},
		{"core ns subresource", "GET", "/api/v1/namespaces/default/pods/web-1/log", "",
			Attributes{Namespace: "default", Resource: "pods", Subresource: "log", Name: "web-1", Verb: "get"}},
		{"apis list", "GET", "/apis/cert-manager.io/v1/namespaces/team-a/certificates", "",
			Attributes{APIGroup: "cert-manager.io", Namespace: "team-a", Resource: "certificates", Verb: "list"}},
		{"watch via query", "GET", "/apis/cert-manager.io/v1/namespaces/team-a/certificates", "watch=true",
			Attributes{APIGroup: "cert-manager.io", Namespace: "team-a", Resource: "certificates", Verb: "watch"}},
		{"create", "POST", "/apis/apps/v1/namespaces/default/deployments", "",
			Attributes{APIGroup: "apps", Namespace: "default", Resource: "deployments", Verb: "create"}},
		{"patch", "PATCH", "/apis/apps/v1/namespaces/default/deployments/web", "",
			Attributes{APIGroup: "apps", Namespace: "default", Resource: "deployments", Name: "web", Verb: "patch"}},
		{"delete", "DELETE", "/apis/apps/v1/namespaces/default/deployments/web", "",
			Attributes{APIGroup: "apps", Namespace: "default", Resource: "deployments", Name: "web", Verb: "delete"}},
		{"deletecollection", "DELETE", "/apis/apps/v1/namespaces/default/deployments", "",
			Attributes{APIGroup: "apps", Namespace: "default", Resource: "deployments", Verb: "deletecollection"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := http.NewRequestWithContext(context.Background(), tc.method, tc.path+"?"+tc.query, http.NoBody)
			require.NoError(t, err)
			got, err := Parse(r)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParse_RejectsMalformed(t *testing.T) {
	for _, path := range []string{"/foo", "/apis//v1/x", "/api"} {
		r, _ := http.NewRequestWithContext(context.Background(), "GET", path, http.NoBody)
		_, err := Parse(r)
		assert.Error(t, err, "expected error for %q", path)
	}
}
