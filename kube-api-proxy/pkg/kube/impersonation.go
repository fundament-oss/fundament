package kube

import (
	"context"
	"net/http"
	"strings"
)

// impersonationContextKey carries the identity the proxy impersonates for one
// request (see WithImpersonation).
type impersonationContextKey struct{}

// Impersonation is an identity the proxy's own ServiceAccount acts as.
type Impersonation struct {
	User   string
	Groups []string
}

// WithImpersonation makes the proxy send Impersonate-User/-Group for this
// request, replacing any the client sent. Only the proxy sets it.
func WithImpersonation(ctx context.Context, imp Impersonation) context.Context {
	return context.WithValue(ctx, impersonationContextKey{}, imp)
}

// ImpersonationFrom returns the Impersonation stored on ctx, if any.
func ImpersonationFrom(ctx context.Context) (Impersonation, bool) {
	imp, ok := ctx.Value(impersonationContextKey{}).(Impersonation)
	return imp, ok
}

// applyImpersonation sets the proxy's own impersonation when the request
// context carries one, after removing every client-supplied Impersonate-*
// header: such a request goes out with the proxy's ServiceAccount, which holds
// impersonation rights the caller does not. Any other request goes out with
// the caller's own ServiceAccount token, so its Impersonate-* headers (kubectl
// --as) pass through untouched and the apiserver authorizes them against the
// caller's own RBAC.
func applyImpersonation(req *http.Request) {
	imp, ok := ImpersonationFrom(req.Context())
	if !ok {
		return
	}
	for key := range req.Header {
		// delete on the raw key: Header.Del canonicalizes its argument and
		// would leave a non-canonical key such as "impersonate-user" behind.
		if isImpersonationHeader(key) {
			delete(req.Header, key)
		}
	}
	req.Header.Set("Impersonate-User", imp.User)
	for _, g := range imp.Groups {
		req.Header.Add("Impersonate-Group", g)
	}
}

// HasClientImpersonation reports whether the client asked to impersonate
// (kubectl --as, --as-group, --as-uid), in any header-name casing.
func HasClientImpersonation(h http.Header) bool {
	for key := range h {
		if isImpersonationHeader(key) {
			return true
		}
	}
	return false
}

func isImpersonationHeader(key string) bool {
	return strings.HasPrefix(strings.ToLower(key), "impersonate-")
}
