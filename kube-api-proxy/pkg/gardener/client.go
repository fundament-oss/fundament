// Package gardener provides a minimal Gardener client for fetching per-cluster
// admin kubeconfigs and issuing ServiceAccount tokens on shoot clusters.
package gardener

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	authenticationv1alpha1 "github.com/gardener/gardener/pkg/apis/authentication/v1alpha1"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	authenticationv1 "k8s.io/api/authentication/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AdminKubeconfig holds the result of an AdminKubeconfigRequest.
type AdminKubeconfig struct {
	Kubeconfig []byte
	ExpiresAt  time.Time
}

const (
	labelClusterID = "fundament.io/cluster-id"

	// fundamentSystemNamespace is the namespace where per-user service accounts live.
	fundamentSystemNamespace = "fundament-system"

	// saTokenExpiry is the requested expiration for SA tokens (15 minutes).
	saTokenExpiry int64 = 900
)

// ErrSyncPending indicates the service account has not been provisioned yet.
var ErrSyncPending = errors.New("service account sync pending")

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
func (c *Client) GetAdminKubeconfig(ctx context.Context, clusterID string, expirationSeconds int64) (*AdminKubeconfig, error) {
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

	return &AdminKubeconfig{
		Kubeconfig: req.Status.Kubeconfig,
		ExpiresAt:  req.Status.ExpirationTimestamp.Time,
	}, nil
}

// SAToken holds a service account token and its expiration.
type SAToken struct {
	Token     string
	ExpiresAt time.Time
}

// RequestSAToken fetches an admin kubeconfig for the cluster, then uses it to
// issue a short-lived ServiceAccount token for the given user on the shoot.
func (c *Client) RequestSAToken(ctx context.Context, clusterID string, userID uuid.UUID) (*SAToken, error) {
	adminKC, err := c.GetAdminKubeconfig(ctx, clusterID, 0)
	if err != nil {
		return nil, fmt.Errorf("get admin kubeconfig: %w", err)
	}

	shootClient, err := clientsetFromKubeconfig(adminKC.Kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("create shoot client: %w", err)
	}

	saName := fmt.Sprintf("fundament-%s", userID)
	expSeconds := saTokenExpiry

	tokenReq := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: &expSeconds,
		},
	}

	result, err := shootClient.CoreV1().ServiceAccounts(fundamentSystemNamespace).CreateToken(
		ctx, saName, tokenReq, metav1.CreateOptions{},
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("service account %s not found: %w", saName, ErrSyncPending)
		}
		return nil, fmt.Errorf("create token for SA %s: %w", saName, err)
	}

	return &SAToken{
		Token:     result.Status.Token,
		ExpiresAt: result.Status.ExpirationTimestamp.Time,
	}, nil
}

func clientsetFromKubeconfig(kubeconfig []byte) (*kubernetes.Clientset, error) {
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}
	cs, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}
	return cs, nil
}
