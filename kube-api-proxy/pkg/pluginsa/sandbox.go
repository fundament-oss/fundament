package pluginsa

import (
	"context"
	"fmt"

	authenticationv1 "k8s.io/api/authentication/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/gardener"
)

// sandboxSATokenExpiry is the requested TokenRequest lifetime in the sandbox.
// Local dev only — kept short so a mis-scoped role change is picked up on the
// next mint.
const sandboxSATokenExpiry int64 = 900

var pluginInstallationGVR = schema.GroupVersionResource{
	Group:    "plugins.fundament.io",
	Version:  "v1",
	Resource: "plugininstallations",
}

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

func (s *sandboxClient) ResolvePluginSA(ctx context.Context, _, installationName string) (*gardener.PluginSAToken, error) {
	dyn, err := dynamic.NewForConfig(s.restConfig)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}

	cr, err := dyn.Resource(pluginInstallationGVR).Get(ctx, installationName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("PluginInstallation %q not found: %w", installationName, gardener.ErrSyncPending)
		}
		return nil, fmt.Errorf("get PluginInstallation %q: %w", installationName, err)
	}

	hash, _ := nestedString(cr.Object, "spec", "definitionRef", "definitionHash")

	kubeClient, err := kubernetes.NewForConfig(s.restConfig)
	if err != nil {
		return nil, fmt.Errorf("create kube client: %w", err)
	}

	saName := "plugin-" + installationName
	saNamespace := "plugin-" + installationName
	exp := sandboxSATokenExpiry
	req := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{ExpirationSeconds: &exp},
	}

	result, err := kubeClient.CoreV1().ServiceAccounts(saNamespace).CreateToken(ctx, saName, req, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("plugin SA %s/%s not found: %w", saNamespace, saName, gardener.ErrSyncPending)
		}
		return nil, fmt.Errorf("create token for plugin SA %s/%s: %w", saNamespace, saName, err)
	}

	return &gardener.PluginSAToken{
		Token:                result.Status.Token,
		ExpiresAt:            result.Status.ExpirationTimestamp.Time,
		PinnedDefinitionHash: hash,
	}, nil
}

func nestedString(obj map[string]any, path ...string) (string, bool) {
	var cur any = obj
	for _, k := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return "", false
		}
		cur, ok = m[k]
		if !ok {
			return "", false
		}
	}
	s, ok := cur.(string)
	return s, ok
}
