package installproxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/fundament-oss/fundament/plugin-proxy/pkg/kube"
)

// ClusterProxyBackend forwards to a plugin's runtime pod or to plugin-controller
// via the target cluster's API-server service proxy. The cluster admin
// credential is used; no per-user or per-plugin token is injected here, because
// these routes carry no kube RBAC scope (FUN-17 "Scoping").
type ClusterProxyBackend struct {
	AdminKubeconfig *kube.AdminKubeconfigCache
}

func (b *ClusterProxyBackend) Serve(w http.ResponseWriter, r *http.Request, route Route) {
	transport, host, err := b.AdminKubeconfig.HTTPClientFor(r.Context(), route.ClusterID)
	if err != nil {
		http.Error(w, fmt.Sprintf("admin kubeconfig: %s", err), http.StatusBadGateway)
		return
	}

	target, err := url.Parse(host)
	if err != nil {
		http.Error(w, "bad upstream host", http.StatusBadGateway)
		return
	}

	tail := (&url.URL{Path: route.RemainingPath}).EscapedPath()
	var upstreamPath string
	switch route.Kind {
	case RouteRuntime:
		ns := "plugin-" + url.PathEscape(route.PluginName)
		upstreamPath = fmt.Sprintf("/api/v1/namespaces/%s/services/runtime:8080/proxy/%s", ns, tail)
	case RouteController:
		upstreamPath = fmt.Sprintf("/api/v1/namespaces/fundament-system/services/plugin-controller:8080/proxy/%s", tail)
	default:
		http.Error(w, fmt.Sprintf("unknown route kind %d", route.Kind), http.StatusInternalServerError)
		return
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			// SetURL appends the inbound path; overwrite to the K8s
			// service-proxy URL. Path is already percent-escaped, so set
			// RawPath too — otherwise URL.String() re-escapes.
			pr.Out.URL.Path = upstreamPath
			pr.Out.URL.RawPath = upstreamPath
			pr.Out.URL.RawQuery = pr.In.URL.RawQuery
			// Do not forward the client's PluginToken or any cookie downstream.
			// Hop-by-hop headers are stripped by ReverseProxy automatically.
			pr.Out.Header.Del("Authorization")
			pr.Out.Header.Del("Cookie")
		},
		Transport: transport,
	}
	proxy.ServeHTTP(w, r)
}
