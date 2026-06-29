package httpx

import (
	"net/http"
	"strings"
)

// WithCORS answers the cross-site preflight for the plugin iframe. The iframe
// runs on the plugin-proxy origin and calls the proxies cross-origin with a
// PluginToken in the Authorization header — a non-simple request that triggers
// a preflight (FUN-17 "CSP and CORS"). The token rides in Authorization, so
// credentials mode stays off — Access-Control-Allow-Credentials is NOT set.
func WithCORS(allowedOrigin string, methods []string, next http.Handler) http.Handler {
	allowMethods := strings.Join(methods, ", ")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") == allowedOrigin {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", allowMethods)
			w.Header().Add("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
