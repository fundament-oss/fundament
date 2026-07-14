// Package gardener issues per-user ServiceAccount tokens on shoot clusters,
// building on the shared Gardener client in common/gardener.
package gardener

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	authenticationv1 "k8s.io/api/authentication/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/fundament-oss/fundament/common/gardener"
)

const (
	// fundamentSystemNamespace is the namespace where per-user service accounts live.
	fundamentSystemNamespace = "fundament-system"

	// saTokenExpiry is the requested expiration for SA tokens (15 minutes).
	saTokenExpiry int64 = 900
)

// ErrSyncPending indicates the service account has not been provisioned yet.
var ErrSyncPending = errors.New("service account sync pending")

// AdminKubeconfig holds the result of an AdminKubeconfigRequest.
type AdminKubeconfig = gardener.AdminKubeconfig

// Client fetches admin kubeconfigs from Gardener and issues per-user SA
// tokens on shoots.
type Client struct {
	*gardener.Client
}

// New creates a Client from the kubeconfig at the given path.
func New(kubeconfigPath string, logger *slog.Logger) (*Client, error) {
	c, err := gardener.New(kubeconfigPath, logger)
	if err != nil {
		return nil, err
	}
	return &Client{Client: c}, nil
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
