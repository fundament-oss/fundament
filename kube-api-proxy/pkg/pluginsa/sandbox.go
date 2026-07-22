package pluginsa

import (
	"context"
	"fmt"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/gardener"
)

// NewSandboxClient returns a Client that reads PluginInstallation CRs and
// mints TokenRequests directly against a kubeconfig loaded from disk. Every
// clusterID is routed to the same cluster (the local plugin sandbox), so this
// exists purely to exercise the FUN-17 plugin-SA flow end-to-end without
// wiring Gardener.
//
// The returned Client satisfies the [Client] interface, so wrapping it in
// [New] yields a real [Resolver] that mints tokens the sandbox apiserver will
// accept — no sentinel handling required downstream.
func NewSandboxClient(kubeconfigPath string) (Client, error) {
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("load sandbox kubeconfig %q: %w", kubeconfigPath, err)
	}
	return &sandboxClient{restConfig: restConfig}, nil
}

type sandboxClient struct {
	restConfig *rest.Config
}

// ResolvePluginSA delegates to the shared resolver so the sandbox matches the
// production path exactly. clusterID is ignored — every cluster is the sandbox.
func (s *sandboxClient) ResolvePluginSA(ctx context.Context, _, installationID, installationName string) (*gardener.PluginSAToken, error) {
	return gardener.ResolvePluginSAFromRESTConfig(ctx, s.restConfig, installationID, installationName)
}
