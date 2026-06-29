package installproxy

import (
	"fmt"
	"net/http"

	"github.com/fundament-oss/fundament/plugin-proxy/pkg/kube"
)

// ClusterProxyBackend forwards to a plugin's runtime pod or to plugin-controller
// via the target cluster's API-server service proxy. The cluster admin
// credential is used; no per-user or per-plugin token is injected here, because
// these routes carry no kube RBAC scope (FUN-17 "Scoping").
type ClusterProxyBackend struct {
	AdminKubeconfig *kube.AdminKubeconfigCache
}

func (b *ClusterProxyBackend) Do(r *http.Request, route Route) (*http.Response, error) {
	transport, host, err := b.AdminKubeconfig.HTTPClientFor(r.Context(), route.ClusterID)
	if err != nil {
		return nil, fmt.Errorf("admin kubeconfig: %w", err)
	}

	var upstreamPath string
	switch route.Kind {
	case RouteRuntime:
		upstreamPath = fmt.Sprintf("/api/v1/namespaces/plugin-%s/services/runtime:8080/proxy/%s",
			route.PluginName, route.RemainingPath)
	case RouteController:
		upstreamPath = fmt.Sprintf("/api/v1/namespaces/fundament-system/services/plugin-controller:8080/proxy/%s",
			route.RemainingPath)
	default:
		return nil, fmt.Errorf("unknown route kind %d", route.Kind)
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, host+upstreamPath, r.Body)
	if err != nil {
		return nil, err
	}
	for k, v := range r.Header {
		// Do not forward the client's PluginToken or any cookie downstream.
		if k == "Authorization" || k == "Cookie" {
			continue
		}
		req.Header[k] = append([]string(nil), v...)
	}
	return (&http.Client{Transport: transport}).Do(req)
}
