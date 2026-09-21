// Package gardener provides a minimal Gardener client for organization-api.
//
// The only call site today is GetCluster, which needs the per-shoot metrics
// dashboard URL stored in the <shoot>.monitoring secret in the project
// namespace of the virtual-garden cluster (see ADR-0025).
package gardener

import (
	"context"
	"crypto/x509"
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

	// caClusterSuffix names the shoot's cluster CA bundle in the project
	// namespace, published as a ConfigMap.
	caClusterSuffix = ".ca-cluster"
	caBundleKey     = "ca.crt"

	// plutonoURLAnnotation is the annotation Gardener sets on the monitoring
	// secret carrying the Plutono dashboard URL.
	plutonoURLAnnotation = "plutono-url"

	// prometheusURLAnnotation carries the per-shoot Prometheus ingress URL.
	// It accepts the same basic-auth credentials as the dashboard and serves
	// the standard Prometheus HTTP API directly — no Plutono involved.
	prometheusURLAnnotation = "prometheus-url"
)

// ErrNotFound is returned when no shoot or monitoring secret exists for the
// requested cluster. Callers should treat this as "URL unavailable", not as
// a hard error.
var ErrNotFound = errors.New("monitoring resource not found")

// MonitoringInfo carries the per-shoot observability endpoints and the
// basic-auth credentials Gardener generates for them.
type MonitoringInfo struct {
	// URL is the Plutono dashboard URL (the user-facing deep link).
	URL string
	// PrometheusURL is the per-shoot Prometheus ingress; empty when the
	// Gardener version does not annotate it.
	PrometheusURL string
	Username      string
	Password      string
	// CABundle is the shoot's cluster CA, which signs its observability
	// ingress certificates. Empty when the shoot publishes none.
	CABundle []byte
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
	caBundle, err := c.caBundle(ctx, shoot.Namespace, shoot.Name)
	if err != nil {
		return nil, err
	}

	return &MonitoringInfo{
		URL:           url,
		PrometheusURL: secret.Annotations[prometheusURLAnnotation],
		Username:      string(secret.Data["username"]),
		Password:      string(secret.Data["password"]),
		CABundle:      caBundle,
	}, nil
}

// caBundle reads the <shoot>.ca-cluster ConfigMap.
//
//   - absent or empty → (nil, nil): provisioning shoot, or a wildcard-cert seed
//   - any read error, forbidden included → error: a forbidden read is a
//     misconfigured Gardener credential, and a guessed "no CA" would be cached
//     for minutes where a failed resolution is retried in seconds
func (c *RealClient) caBundle(ctx context.Context, namespace, shootName string) ([]byte, error) {
	key := types.NamespacedName{Namespace: namespace, Name: shootName + caClusterSuffix}

	configMap := &corev1.ConfigMap{}
	if err := c.client.Get(ctx, key, configMap); err != nil {
		switch {
		case apierrors.IsNotFound(err):
			c.logger.DebugContext(ctx, "shoot publishes no cluster CA",
				"namespace", key.Namespace, "name", key.Name)
			return nil, nil
		case apierrors.IsForbidden(err):
			return nil, fmt.Errorf("read shoot cluster CA %s/%s: grant get on configmaps to the Gardener credential: %w",
				key.Namespace, key.Name, err)
		default:
			return nil, fmt.Errorf("read shoot cluster CA %s/%s: %w", key.Namespace, key.Name, err)
		}
	}

	pem := []byte(configMap.Data[caBundleKey])
	if len(pem) == 0 {
		return nil, nil
	}
	if !x509.NewCertPool().AppendCertsFromPEM(pem) {
		c.logger.WarnContext(ctx, "shoot cluster CA holds no certificate",
			"namespace", key.Namespace, "name", key.Name)
		return nil, nil
	}
	return pem, nil
}

// NoopClient is the zero-config implementation used when no Gardener
// kubeconfig is wired in (mock mode, local dev without Gardener).
type NoopClient struct{}

// Monitoring always returns ErrNotFound.
func (NoopClient) Monitoring(context.Context, uuid.UUID) (*MonitoringInfo, error) {
	return nil, ErrNotFound
}
