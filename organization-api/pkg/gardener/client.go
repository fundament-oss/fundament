// Package gardener provides a minimal Gardener client for organization-api.
//
// The only call site today is GetCluster, which needs the per-shoot metrics
// dashboard URL stored in the <shoot>.monitoring secret in the project
// namespace of the virtual-garden cluster (see ADR-0025).
package gardener

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// labelClusterID matches the label cluster-worker sets on every Shoot it
	// creates (see cluster-worker/pkg/client/gardener/labels.go). Using the
	// label avoids re-deriving the shoot name on the org-api side.
	labelClusterID = "fundament.io/cluster-id"

	// monitoringSecretSuffix is the suffix Gardener uses for the per-shoot
	// monitoring credentials secret: "<shoot-name>.monitoring".
	monitoringSecretSuffix = ".monitoring"

	// plutonoURLAnnotation is the annotation Gardener sets on the monitoring
	// secret carrying the Plutono dashboard URL.
	plutonoURLAnnotation = "plutono-url"
)

// ErrNotFound is returned when no shoot or monitoring secret exists for the
// requested cluster. Callers should treat this as "URL unavailable", not as
// a hard error.
var ErrNotFound = errors.New("monitoring resource not found")

// MonitoringInfo carries the per-shoot Plutono dashboard URL and the
// basic-auth credentials Gardener generates for it.
type MonitoringInfo struct {
	URL      string
	Username string
	Password string
}

// Client looks up Gardener-side artifacts for a given cluster.
type Client interface {
	// Monitoring returns the per-shoot Plutono URL and basic-auth credentials,
	// or ErrNotFound if the shoot or secret is not yet available.
	Monitoring(ctx context.Context, clusterID uuid.UUID) (*MonitoringInfo, error)
}

// RealClient talks to the virtual-garden cluster.
type RealClient struct {
	client client.Client
	logger *slog.Logger
}

// NewReal builds a RealClient from a kubeconfig path. An empty path falls back
// to in-cluster config.
func NewReal(kubeconfigPath string, logger *slog.Logger) (*RealClient, error) {
	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	if kubeconfigPath == "" {
		loadingRules = clientcmd.NewDefaultClientConfigLoadingRules()
	}
	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, nil)
	cfg, err := cc.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load gardener kubeconfig: %w", err)
	}

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("add core scheme: %w", err)
	}
	if err := gardencorev1beta1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("add gardener core scheme: %w", err)
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("create gardener client: %w", err)
	}

	logger.Info("connected to Gardener API", "host", cfg.Host)
	return &RealClient{client: c, logger: logger}, nil
}

// Monitoring finds the Shoot for clusterID, reads its monitoring secret, and
// returns the URL + basic-auth credentials. Returns ErrNotFound when the
// shoot or secret does not exist yet.
func (c *RealClient) Monitoring(ctx context.Context, clusterID uuid.UUID) (*MonitoringInfo, error) {
	shootList := &gardencorev1beta1.ShootList{}
	if err := c.client.List(ctx, shootList,
		client.MatchingLabels{labelClusterID: clusterID.String()},
	); err != nil {
		return nil, fmt.Errorf("list shoots: %w", err)
	}
	if len(shootList.Items) == 0 {
		return nil, ErrNotFound
	}
	shoot := &shootList.Items[0]

	secret := &corev1.Secret{}
	key := types.NamespacedName{
		Namespace: shoot.Namespace,
		Name:      shoot.Name + monitoringSecretSuffix,
	}
	if err := c.client.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get monitoring secret %s/%s: %w", key.Namespace, key.Name, err)
	}

	url := secret.Annotations[plutonoURLAnnotation]
	if url == "" {
		return nil, ErrNotFound
	}
	return &MonitoringInfo{
		URL:      url,
		Username: string(secret.Data["username"]),
		Password: string(secret.Data["password"]),
	}, nil
}

// NoopClient is the zero-config implementation used when no Gardener
// kubeconfig is wired in (mock mode, local dev without Gardener).
type NoopClient struct{}

// Monitoring always returns ErrNotFound.
func (NoopClient) Monitoring(context.Context, uuid.UUID) (*MonitoringInfo, error) {
	return nil, ErrNotFound
}
