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
		{"watch via query truthy 1", "GET", "/api/v1/pods", "watch=1",
			Attributes{Resource: "pods", Verb: "watch"}},
		{"watch via query falsy 0", "GET", "/api/v1/pods", "watch=0",
			Attributes{Resource: "pods", Verb: "list"}},
		{"watch via query non-bool truthy", "GET", "/api/v1/pods", "watch=yes",
			Attributes{Resource: "pods", Verb: "watch"}},
		{"watch via query arbitrary value", "GET", "/api/v1/pods", "watch=maybe",
			Attributes{Resource: "pods", Verb: "watch"}},
		{"watch via query FALSE", "GET", "/api/v1/pods", "watch=FALSE",
			Attributes{Resource: "pods", Verb: "list"}},
		{"namespace status subresource", "GET", "/api/v1/namespaces/default/status", "",
			Attributes{Resource: "namespaces", Name: "default", Subresource: "status", Verb: "get"}},
		{"namespace finalize subresource", "PUT", "/api/v1/namespaces/default/finalize", "",
			Attributes{Resource: "namespaces", Name: "default", Subresource: "finalize", Verb: "update"}},
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

// FuzzParse feeds arbitrary methods and paths at Parse. Because the parsed
// Attributes drive the SubjectAccessReview, a malformed request must never
// panic and must never yield attributes that would authorize against an empty
// resource/verb. Parse either returns an error or a fully-populated decision.
func FuzzParse(f *testing.F) {
	seeds := []struct{ method, target string }{
		{"GET", "/api/v1/pods"},
		{"GET", "/api/v1/namespaces/default/pods/web-1/log"},
		{"GET", "/apis/cert-manager.io/v1/namespaces/team-a/certificates?watch=true"},
		{"DELETE", "/apis/apps/v1/namespaces/default/deployments"},
		{"POST", "/api/v1/namespaces"},
		{"GET", "/foo"},
		{"GET", "/apis//v1/x"},
		{"", ""},
	}
	for _, s := range seeds {
		f.Add(s.method, s.target)
	}

	f.Fuzz(func(t *testing.T, method, target string) {
		r, err := http.NewRequestWithContext(context.Background(), method, target, http.NoBody)
		if err != nil {
			return // not a request Parse would ever receive
		}
		got, err := Parse(r)
		if err != nil {
			return
		}
		// A nil error means Parse authorized a concrete decision; the SAR would
		// be meaningless (and dangerously broad) without both fields.
		assert.NotEmpty(t, got.Resource, "resource empty for %q %q", method, target)
		assert.NotEmpty(t, got.Verb, "verb empty for %q %q", method, target)
	})
}
