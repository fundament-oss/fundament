package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	authorizationv1 "k8s.io/api/authorization/v1"

	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kube"
)

// GUI tools ask a SelfSubjectAccessReview before they list namespaces (k9s:
// client.ValidNamespaceNames -> CanI), and hide the namespace view when the
// shoot answers no. The shoot always answers no for a project member, because
// the listing is served by the proxy (see namespaces.go), not granted by RBAC.
// So the proxy forwards the review as the user and turns a denial for exactly
// list/watch on namespaces at cluster scope into an allow. Every other review
// passes through untouched.

const accessReviewPath = "/apis/authorization.k8s.io/v1/selfsubjectaccessreviews"

// namespaceListAllowedReason explains the rewritten decision to the client.
const namespaceListAllowedReason = "kube-api-proxy serves namespace listings filtered to the projects you can view"

func isSelfSubjectAccessReview(r *http.Request) bool {
	return r.Method == http.MethodPost && r.URL.Path == accessReviewPath
}

// serveAccessReview forwards the review as the user (the request context
// already carries the user's SA token) and patches a namespace-list denial.
func (n *namespaceLister) serveAccessReview(w http.ResponseWriter, r *http.Request, clusterID uuid.UUID) {
	ctx := context.WithValue(r.Context(), kube.ClusterIDContextKey{}, clusterID.String())
	r = r.Clone(ctx)
	// JSON so the decision can be read and rewritten; client-go decodes by the
	// response Content-Type, so a client that asked for protobuf still copes.
	r.Header.Set("Accept", "application/json")
	r.Header.Del("Accept-Encoding")

	buf := &bufferedResponse{header: http.Header{}, status: http.StatusOK}
	n.upstream.ServeHTTP(buf, r)

	body := buf.body.Bytes()
	if buf.status/100 == 2 {
		patched, ok := allowNamespaceList(body)
		if ok {
			body = patched
			n.logger.InfoContext(ctx, "allowed namespace list access review", "cluster_id", clusterID)
		}
	}

	maps.Copy(w.Header(), buf.header)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(buf.status)
	_, _ = w.Write(body)
}

// allowNamespaceList rewrites a denied review of list/watch on namespaces to
// allowed. Namespace and name are ignored: a listing is served (filtered, maybe
// empty) whatever they say, and clients do send them — kubectl auth can-i its
// current namespace, k9s ns=name=<current namespace> for its namespace view.
// get stays with RBAC (the user's RoleBindings). It reports false, leaving the
// body alone, for any other review.
func allowNamespaceList(body []byte) ([]byte, bool) {
	var sar authorizationv1.SelfSubjectAccessReview
	err := json.Unmarshal(body, &sar)
	if err != nil {
		return nil, false
	}
	ra := sar.Spec.ResourceAttributes
	if ra == nil || sar.Status.Allowed ||
		ra.Group != "" || ra.Resource != "namespaces" || ra.Subresource != "" ||
		(ra.Verb != "list" && ra.Verb != "watch") {
		return nil, false
	}
	sar.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: true, Reason: namespaceListAllowedReason}
	out, err := json.Marshal(&sar)
	if err != nil {
		return nil, false
	}
	return out, true
}

// bufferedResponse captures a small upstream response so it can be rewritten.
type bufferedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (b *bufferedResponse) Header() http.Header { return b.header }

func (b *bufferedResponse) WriteHeader(code int) { b.status = code }

func (b *bufferedResponse) Write(p []byte) (int, error) {
	n, err := b.body.Write(p)
	if err != nil {
		return n, fmt.Errorf("buffer access review response: %w", err)
	}
	return n, nil
}
