package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

	// Dropped from a write it would leave a delete or patch unscoped.
	if r.Method != http.MethodGet {
		return uuid.Nil, false, errors.New(projectQueryParam + " is only supported on GET")
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
	// The namespaces are the caller's; the lists would run as someone else.
	case kube.HasClientImpersonation(r.Header):
		return uuid.Nil, false, errors.New(projectQueryParam + " does not support impersonation")
	}
	return projectID, true, nil
}

// unsupportedClusterList reports whether path is a cluster-wide collection
// (no namespace, no object name) other than /apis/{g}/{v}/{plural}: core
// /api/v1/{plural}, or discovery such as /api/v1 and /apis/{g}/{v}.
func unsupportedClusterList(path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if slices.Contains(parts, "") {
		return false
	}
	switch parts[0] {
	case "api", "apis":
		return len(parts) <= 3
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

const (
	// projectListParallelism bounds the namespace lists in flight at once.
	projectListParallelism = 4
	// maxProjectListBytes caps the namespace lists held at once: limit and
	// continue are refused, so the client cannot bound the list itself.
	maxProjectListBytes = 32 << 20
)

var errProjectListTooLarge = errors.New("project list too large")

// cappedResponse is a bufferedResponse whose writes draw on a budget shared
// by all of a project list's namespaces.
type cappedResponse struct {
	bufferedResponse

	budget   *atomic.Int64
	exceeded func()
}

func (c *cappedResponse) Write(p []byte) (int, error) {
	if c.budget.Add(int64(len(p))) > maxProjectListBytes {
		c.exceeded()
		return 0, errProjectListTooLarge
	}
	return c.bufferedResponse.Write(p)
}

// serveProjectList answers the cluster-wide list r with the items of each
// namespace, fetched through forward. A namespace that answers 403 is left
// out with a Warning header: its RoleBinding may not have landed yet. Any
// other failed sub-request fails the whole, as does every namespace being
// forbidden.
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

	results, tooLarge := fetchNamespaceLists(r, parts, namespaces, forward)
	if tooLarge {
		kube.WriteStatus(w, http.StatusInternalServerError, metav1.StatusReasonInternalError,
			fmt.Sprintf("the project list is larger than %d MiB; list per namespace instead", maxProjectListBytes>>20))
		return
	}

	var apiVersion, kind string
	var items []json.RawMessage
	var forbidden []int
	for i, ns := range namespaces {
		buf := results[i]
		switch {
		case buf == nil: // aborted, or r's context ended first
			http.Error(w, "the list of namespace "+ns+" was aborted", http.StatusServiceUnavailable)
			return
		case buf.status == http.StatusForbidden:
			forbidden = append(forbidden, i)
			continue
		case buf.status/100 != 2:
			writeBuffered(w, &buf.bufferedResponse)
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
		results[i] = nil // the items are copied out; let the body go
		if apiVersion == "" {
			apiVersion, kind = list.APIVersion, list.Kind
		}
		items = append(items, list.Items...)
	}
	if len(forbidden) > 0 && len(forbidden) == len(namespaces) {
		writeBuffered(w, &results[forbidden[0]].bufferedResponse)
		return
	}
	for _, i := range forbidden {
		w.Header().Add("Warning", fmt.Sprintf("299 - %q", "namespace "+namespaces[i]+" left out: forbidden"))
	}
	writeMergedList(w, apiVersion, kind, items)
}

// fetchNamespaceLists forwards the list to each namespace, a few at a time.
// A nil result is a namespace without an answer: the budget ran out first,
// which tooLarge reports, or r's context ended.
func fetchNamespaceLists(r *http.Request, parts, namespaces []string, forward func(http.ResponseWriter, *http.Request)) ([]*cappedResponse, bool) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	var budget atomic.Int64
	var tooLarge atomic.Bool
	exceeded := func() {
		tooLarge.Store(true)
		cancel()
	}

	results := make([]*cappedResponse, len(namespaces))
	sem := make(chan struct{}, projectListParallelism)
	var wg sync.WaitGroup
	var panicked atomic.Value
	for i, ns := range namespaces {
		wg.Go(func() {
			// ReverseProxy panics with ErrAbortHandler when it cannot copy the
			// body, as when the budget runs out. Only the server's own handler
			// goroutine recovers panics, so they are caught here; anything else
			// is raised again on that goroutine below.
			defer func() {
				if p := recover(); p != nil && p != http.ErrAbortHandler { //nolint:errorlint // ReverseProxy panics with the sentinel itself
					panicked.CompareAndSwap(nil, p)
				}
			}()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			sub := r.Clone(ctx)
			sub.URL = cloneURL(r.URL)
			sub.URL.Path = "/apis/" + parts[0] + "/" + parts[1] + "/namespaces/" + ns + "/" + parts[2]
			sub.URL.RawPath = ""
			sub.Header.Set("Accept", "application/json")
			sub.Header.Del("Accept-Encoding")

			buf := &cappedResponse{
				bufferedResponse: bufferedResponse{header: http.Header{}, status: http.StatusOK},
				budget:           &budget,
				exceeded:         exceeded,
			}
			forward(buf, sub)
			results[i] = buf
		})
	}
	wg.Wait()
	if p := panicked.Load(); p != nil {
		panic(p)
	}
	return results, tooLarge.Load()
}

func writeBuffered(w http.ResponseWriter, buf *bufferedResponse) {
	maps.Copy(w.Header(), buf.header)
	w.WriteHeader(buf.status)
	_, _ = w.Write(buf.body.Bytes())
}

// writeMergedList writes the list piece by piece rather than marshalling a
// second copy of every item.
func writeMergedList(w http.ResponseWriter, apiVersion, kind string, items []json.RawMessage) {
	var head strings.Builder
	head.WriteString("{")
	if apiVersion != "" {
		v, _ := json.Marshal(apiVersion)
		head.WriteString(`"apiVersion":` + string(v) + ",")
	}
	if kind != "" {
		k, _ := json.Marshal(kind)
		head.WriteString(`"kind":` + string(k) + ",")
	}
	head.WriteString(`"metadata":{},"items":[`)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, head.String())
	for i, item := range items {
		if i > 0 {
			_, _ = io.WriteString(w, ",")
		}
		_, _ = w.Write(item)
	}
	_, _ = io.WriteString(w, "]}")
}

// writeProjectNamespacesError answers a failed projectNamespaces lookup.
func writeProjectNamespacesError(w http.ResponseWriter, err error) {
	if errors.Is(err, errProxyCredentials) {
		kube.WriteStepError(w, kube.StepProxyCredentials, err)
		return
	}
	http.Error(w, "internal server error", http.StatusInternalServerError)
}
