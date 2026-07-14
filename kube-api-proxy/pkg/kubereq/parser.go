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
	//
	// Exception: the apiserver special-cases /api/v1/namespaces/{name}/status
	// and /finalize as subresources OF the namespace (resource "namespaces",
	// subresource "status"/"finalize"), not as a namespace-scoped resource.
	// Mirror that so the SAR checks the same attributes the apiserver enforces.
	if parts[i] == "namespaces" && i+2 < len(parts) &&
		!(i+3 == len(parts) && (parts[i+2] == "status" || parts[i+2] == "finalize")) {
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
	// Any tail segments after the subresource (e.g. /pods/x/proxy/some/path)
	// are irrelevant to SAR attributes and dropped.
	if i < len(parts) {
		out.Subresource = parts[i]
	}

	out.Verb = inferVerb(r, out.Name != "")
	return out, nil
}

func inferVerb(r *http.Request, hasName bool) string {
	switch strings.ToUpper(r.Method) {
	case http.MethodGet:
		// Match the apiserver's query-param bool conversion exactly
		// (apimachinery Convert_Slice_string_To_bool): the param resolves to
		// false ONLY when absent, "0", or case-insensitive "false"; every other
		// present value ("yes", "maybe", even bare "?watch") is a watch. Using
		// strconv.ParseBool here would SAR-check "list" for e.g. "?watch=yes"
		// while the apiserver performs a watch — letting a user with list-but-
		// not-watch access watch.
		if vs, ok := r.URL.Query()["watch"]; ok && len(vs) > 0 {
			if v := vs[0]; v != "0" && !strings.EqualFold(v, "false") {
				return "watch"
			}
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
