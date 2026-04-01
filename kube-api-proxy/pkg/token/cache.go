// Package token manages per-user ServiceAccount tokens for proxied cluster access.
package token

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
	authenticationv1 "k8s.io/api/authentication/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/gardener"
)

const (
	// saTokenExpiry is the requested expiration for SA tokens (15 minutes).
	saTokenExpiry int64 = 900

	// refreshRatio is the fraction of TTL at which to proactively refresh (80%).
	refreshRatio = 0.8

	// fundamentSystemNamespace is the namespace where service accounts live.
	fundamentSystemNamespace = "fundament-system"
)

// ErrSyncPending indicates the service account has not been provisioned yet.
var ErrSyncPending = errors.New("service account sync pending")

// cacheKey uniquely identifies a cached token.
type cacheKey struct {
	userID    uuid.UUID
	clusterID string
}

type cachedToken struct {
	token     string
	expiresAt time.Time
	issuedAt  time.Time
}

// shouldRefresh returns true if the token has passed refreshRatio of its TTL.
func (ct *cachedToken) shouldRefresh() bool {
	ttl := ct.expiresAt.Sub(ct.issuedAt)
	refreshAt := ct.issuedAt.Add(time.Duration(float64(ttl) * refreshRatio))
	return time.Now().After(refreshAt)
}

// isExpired returns true if the token has expired.
func (ct *cachedToken) isExpired() bool {
	return time.Now().After(ct.expiresAt)
}

// Cache manages per-(user, cluster) SA tokens with proactive refresh.
type Cache struct {
	gardener *gardener.Client
	logger   *slog.Logger
	tokens   sync.Map           // cacheKey → *cachedToken
	group    singleflight.Group // deduplicates concurrent token requests
}

// NewCache creates a new token cache.
func NewCache(gc *gardener.Client, logger *slog.Logger) *Cache {
	return &Cache{
		gardener: gc,
		logger:   logger,
	}
}

// GetToken returns a valid SA token for the given user and cluster.
// It uses a cached token if available and not expired, triggering an async
// refresh if the token has passed 80% of its TTL.
func (c *Cache) GetToken(ctx context.Context, userID uuid.UUID, clusterID string) (string, error) {
	key := cacheKey{userID: userID, clusterID: clusterID}

	if v, ok := c.tokens.Load(key); ok {
		ct := v.(*cachedToken)
		if !ct.isExpired() {
			if ct.shouldRefresh() {
				// Proactive async refresh — don't block the request.
				go c.refresh(context.WithoutCancel(ctx), key)
			}
			return ct.token, nil
		}
	}

	// Cache miss or expired — synchronous fetch.
	return c.fetchAndCache(ctx, key)
}

// ForceRefresh evicts the cached token and fetches a new one.
func (c *Cache) ForceRefresh(ctx context.Context, userID uuid.UUID, clusterID string) (string, error) {
	key := cacheKey{userID: userID, clusterID: clusterID}
	c.tokens.Delete(key)
	return c.fetchAndCache(ctx, key)
}

func (c *Cache) fetchAndCache(ctx context.Context, key cacheKey) (string, error) {
	sfKey := fmt.Sprintf("%s:%s", key.userID, key.clusterID)

	v, err, _ := c.group.Do(sfKey, func() (any, error) {
		return c.requestToken(ctx, key)
	})
	if err != nil {
		return "", err
	}

	return v.(string), nil
}

func (c *Cache) refresh(ctx context.Context, key cacheKey) {
	sfKey := fmt.Sprintf("%s:%s", key.userID, key.clusterID)

	_, err, _ := c.group.Do(sfKey, func() (any, error) {
		return c.requestToken(ctx, key)
	})
	if err != nil {
		c.logger.WarnContext(ctx, "async token refresh failed",
			"user_id", key.userID, "cluster_id", key.clusterID, "error", err)
	}
}

func (c *Cache) requestToken(ctx context.Context, key cacheKey) (string, error) {
	// Get admin kubeconfig for the cluster.
	adminKC, err := c.gardener.GetAdminKubeconfig(ctx, key.clusterID, 0)
	if err != nil {
		return "", fmt.Errorf("get admin kubeconfig: %w", err)
	}

	shootClient, err := clientsetFromKubeconfig(adminKC.Kubeconfig)
	if err != nil {
		return "", fmt.Errorf("create shoot client: %w", err)
	}

	saName := fmt.Sprintf("fundament-%s", key.userID)
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
			return "", fmt.Errorf("service account %s not found: %w", saName, ErrSyncPending)
		}
		return "", fmt.Errorf("create token for SA %s: %w", saName, err)
	}

	// Use the actual expiration from the API server response, not the requested value.
	now := time.Now()
	ct := &cachedToken{
		token:     result.Status.Token,
		expiresAt: result.Status.ExpirationTimestamp.Time,
		issuedAt:  now,
	}
	c.tokens.Store(key, ct)

	c.logger.InfoContext(ctx, "SA token issued",
		"user_id", key.userID, "cluster_id", key.clusterID,
		"expires_at", ct.expiresAt)

	return ct.token, nil
}

func clientsetFromKubeconfig(kubeconfig []byte) (*kubernetes.Clientset, error) {
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}
	return kubernetes.NewForConfig(restConfig)
}
