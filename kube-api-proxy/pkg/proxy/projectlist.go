package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kube"
)

// Project lists: the console lists a namespaced plugin resource for one
// project. Kubernetes can't select by namespace label, so the proxy fans a
// cluster-wide list out to each of the project's namespaces, each through the
// caller's normal auth path, and merges the items.

// projectQueryParam selects a project list; never forwarded to Kubernetes.
const projectQueryParam = "fundamentProjectId"

// parseProjectList strips projectQueryParam from r and reports whether r is a
// project list: a GET on /apis/{g}/{v}/{plural} carrying the parameter.
func parseProjectList(r *http.Request) (uuid.UUID, bool, error) {
	q := r.URL.Query()
	if !q.Has(projectQueryParam) {
		return uuid.Nil, false, nil
	}
	raw := q.Get(projectQueryParam)
	q.Del(projectQueryParam)
	r.URL.RawQuery = q.Encode()

	if r.Method != http.MethodGet {
		return uuid.Nil, false, nil
	}
	if clusterCollectionPath(r.URL.Path) == nil {
		if unsupportedClusterList(r.URL.Path) {
			return uuid.Nil, false, errors.New(projectQueryParam + " is only supported on /apis/{group}/{version}/{plural} lists")
		}
		return uuid.Nil, false, nil
	}
	projectID, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("invalid %s: %w", projectQueryParam, err)
	}
	// One merged page: no watch, no paging.
	switch {
	case isWatch(r):
		return uuid.Nil, false, errors.New(projectQueryParam + " does not support watch")
	case q.Has("limit"), q.Has("continue"):
		return uuid.Nil, false, errors.New(projectQueryParam + " does not support limit or continue")
	}
	return projectID, true, nil
}

// unsupportedClusterList reports whether path is a cluster-wide collection
// (no namespace, no object name) other than /apis/{g}/{v}/{plural}: core
// /api/v1/{plural} or group discovery /apis/{g}/{v}.
func unsupportedClusterList(path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	switch parts[0] {
	case "api":
		return len(parts) == 3
	case "apis":
		return len(parts) == 3 && !slices.Contains(parts, "")
	}
	return false
}

// clusterCollectionPath splits /apis/{g}/{v}/{plural} into its three parts;
// nil for any other path.
func clusterCollectionPath(path string) []string {
	parts := strings.Split(strings.TrimPrefix(path, "/apis/"), "/")
	if !strings.HasPrefix(path, "/apis/") || len(parts) != 3 || slices.Contains(parts, "") {
		return nil
	}
	return parts
}

// serveProjectList answers the cluster-wide list r with the items of each
// namespace, fetched through forward. Any failed sub-request fails the whole.
func serveProjectList(w http.ResponseWriter, r *http.Request, namespaces []string, forward func(http.ResponseWriter, *http.Request)) {
	if accept := r.Header.Get("Accept"); strings.Contains(accept, "as=Table") || strings.Contains(accept, "protobuf") {
		http.Error(w, projectQueryParam+" lists are JSON only", http.StatusNotAcceptable)
		return
	}
	parts := clusterCollectionPath(r.URL.Path)
	if parts == nil {
		http.Error(w, "not a cluster-wide list", http.StatusBadRequest)
		return
	}

	merged := struct {
		APIVersion string            `json:"apiVersion,omitempty"`
		Kind       string            `json:"kind,omitempty"`
		Metadata   struct{}          `json:"metadata"`
		Items      []json.RawMessage `json:"items"`
	}{Items: []json.RawMessage{}}

	for i, ns := range namespaces {
		sub := r.Clone(r.Context())
		sub.URL = cloneURL(r.URL)
		sub.URL.Path = "/apis/" + parts[0] + "/" + parts[1] + "/namespaces/" + ns + "/" + parts[2]
		sub.URL.RawPath = ""
		sub.Header.Set("Accept", "application/json")
		sub.Header.Del("Accept-Encoding")

		buf := &bufferedResponse{header: http.Header{}, status: http.StatusOK}
		forward(buf, sub)
		if buf.status/100 != 2 {
			maps.Copy(w.Header(), buf.header)
			w.WriteHeader(buf.status)
			_, _ = w.Write(buf.body.Bytes())
			return
		}

		var list struct {
			APIVersion string            `json:"apiVersion"`
			Kind       string            `json:"kind"`
			Items      []json.RawMessage `json:"items"`
		}
		if err := json.Unmarshal(buf.body.Bytes(), &list); err != nil {
			http.Error(w, "invalid list response from namespace "+ns, http.StatusBadGateway)
			return
		}
		if i == 0 {
			merged.APIVersion, merged.Kind = list.APIVersion, list.Kind
		}
		merged.Items = append(merged.Items, list.Items...)
	}

	body, err := json.Marshal(merged)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// writeProjectNamespacesError answers a failed projectNamespaces lookup.
func writeProjectNamespacesError(w http.ResponseWriter, err error) {
	if errors.Is(err, errProxyCredentials) {
		kube.WriteStepError(w, kube.StepProxyCredentials, err)
		return
	}
	http.Error(w, "internal server error", http.StatusInternalServerError)
}
