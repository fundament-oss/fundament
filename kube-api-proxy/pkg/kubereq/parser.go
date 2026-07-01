// Package kubereq parses a Kubernetes API request into the attributes a
// SubjectAccessReview needs. It does NOT make an authorization decision — that
// is the cluster's RBAC engine (plugin half) and the gateway's SAR (user half).
package kubereq

import (
	"fmt"
	"net/http"
	"strings"
)

// Attributes is the parsed shape of a Kubernetes API request.
type Attributes struct {
	APIGroup    string // "" for the core group
	Resource    string
	Subresource string
	Name        string
	Namespace   string
	Verb        string
}

// Parse parses r.URL.Path (the canonical kube path, with the /clusters/{id}
// prefix already stripped) and r.Method into Attributes.
func Parse(r *http.Request) (Attributes, error) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		return Attributes{}, fmt.Errorf("path too short: %q", r.URL.Path)
	}

	var out Attributes
	var i int // index of the first resource-grammar element
	switch parts[0] {
	case "api": // /api/v1/...
		i = 2
	case "apis": // /apis/{group}/{version}/...
		if len(parts) < 4 || parts[1] == "" {
			return Attributes{}, fmt.Errorf("apis path missing group: %q", r.URL.Path)
		}
		out.APIGroup = parts[1]
		i = 3
	default:
		return Attributes{}, fmt.Errorf("expected /api or /apis, got %q", parts[0])
	}

	// "namespaces" is both a resource (get/update/delete a Namespace at
	// /api/v1/namespaces/{name}) and the namespace-scope marker for downstream
	// resources (/api/v1/namespaces/{ns}/{resource}...). Only treat it as the
	// scope marker when at least one more segment follows the namespace name.
	if parts[i] == "namespaces" && i+2 < len(parts) {
		out.Namespace = parts[i+1]
		i += 2
	}
	if i >= len(parts) || parts[i] == "" {
		return Attributes{}, fmt.Errorf("missing resource: %q", r.URL.Path)
	}
	out.Resource = parts[i]
	i++
	if i < len(parts) {
		out.Name = parts[i]
		i++
	}
	if i < len(parts) {
		out.Subresource = parts[i]
	}

	out.Verb = inferVerb(r, out.Name != "")
	return out, nil
}

func inferVerb(r *http.Request, hasName bool) string {
	switch strings.ToUpper(r.Method) {
	case http.MethodGet:
		if r.URL.Query().Get("watch") == "true" {
			return "watch"
		}
		if hasName {
			return "get"
		}
		return "list"
	case http.MethodPost:
		return "create"
	case http.MethodPut:
		return "update"
	case http.MethodPatch:
		return "patch"
	case http.MethodDelete:
		if hasName {
			return "delete"
		}
		return "deletecollection"
	default:
		return strings.ToLower(r.Method)
	}
}
