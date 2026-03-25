// Package gardener provides a minimal Gardener client for fetching per-cluster
// admin kubeconfigs via the adminkubeconfig subresource.
package gardener

import (
	"context"
	"fmt"
	"log/slog"

	authenticationv1alpha1 "github.com/gardener/gardener/pkg/apis/authentication/v1alpha1"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const labelClusterID = "fundament.io/cluster-id"

// Client fetches admin kubeconfigs from Gardener for a given cluster ID.
type Client struct {
	client client.Client
	logger *slog.Logger
}

// New creates a Client from the kubeconfig at the given path.
func New(kubeconfigPath string, logger *slog.Logger) (*Client, error) {
	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, nil)
	cfg, err := cc.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load gardener kubeconfig: %w", err)
	}

	scheme := runtime.NewScheme()
	if err := gardencorev1beta1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("add gardener core scheme: %w", err)
	}
	if err := authenticationv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("add gardener authentication scheme: %w", err)
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("create gardener client: %w", err)
	}

	logger.Info("connected to Gardener API", "host", cfg.Host)
	return &Client{client: c, logger: logger}, nil
}

// GetAdminKubeconfig finds the Shoot for clusterID and returns a short-lived
// admin kubeconfig via the Gardener adminkubeconfig subresource.
// Pass expirationSeconds=0 to use the Gardener default (typically 1 hour).
func (c *Client) GetAdminKubeconfig(ctx context.Context, clusterID string, expirationSeconds int64) ([]byte, error) {
	shootList := &gardencorev1beta1.ShootList{}
	if err := c.client.List(ctx, shootList,
		client.MatchingLabels{labelClusterID: clusterID},
	); err != nil {
		return nil, fmt.Errorf("list shoots for cluster %s: %w", clusterID, err)
	}

	if len(shootList.Items) == 0 {
		return nil, fmt.Errorf("no shoot found for cluster %s", clusterID)
	}

	shoot := &shootList.Items[0]
	req := &authenticationv1alpha1.AdminKubeconfigRequest{}
	if expirationSeconds > 0 {
		req.Spec.ExpirationSeconds = &expirationSeconds
	}

	if err := c.client.SubResource("adminkubeconfig").Create(ctx, shoot, req); err != nil {
		return nil, fmt.Errorf("request admin kubeconfig for cluster %s: %w", clusterID, err)
	}

	return req.Status.Kubeconfig, nil
}
