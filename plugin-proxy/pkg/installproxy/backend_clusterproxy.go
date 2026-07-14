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
	AdminKubeconfig kube.KubeconfigSource
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

	tailEscaped := (&url.URL{Path: route.RemainingPath}).EscapedPath()
	var pathDecoded, pathEscaped string
	switch route.Kind {
	case RouteRuntime:
		// plugin-controller names the namespace and Service plugin-<name>
		// (childName in plugin-controller/pkg/controller/resources.go).
		ns := "plugin-" + route.PluginName
		pathDecoded = fmt.Sprintf("/api/v1/namespaces/%s/services/%s:8080/proxy/%s", ns, ns, route.RemainingPath)
		pathEscaped = fmt.Sprintf("/api/v1/namespaces/%s/services/%s:8080/proxy/%s", ns, ns, tailEscaped)
	case RouteController:
		pathDecoded = fmt.Sprintf("/api/v1/namespaces/fundament-system/services/plugin-controller:8080/proxy/%s", route.RemainingPath)
		pathEscaped = fmt.Sprintf("/api/v1/namespaces/fundament-system/services/plugin-controller:8080/proxy/%s", tailEscaped)
	default:
		panic(fmt.Sprintf("unhandled route kind %d", route.Kind))
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			// SetURL appends the inbound path; overwrite to the K8s
			// service-proxy URL. Path is the decoded form and RawPath the
			// escaped form so url.URL.String() emits pathEscaped verbatim
			// (net/url only trusts RawPath when unescape(RawPath) == Path).
			pr.Out.URL.Path = pathDecoded
			pr.Out.URL.RawPath = pathEscaped
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
