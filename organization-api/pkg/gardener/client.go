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
	// namespace, published as both a ConfigMap and a deprecated Secret.
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

// caBundle reads <shoot>.ca-cluster: ConfigMap first, Secret on any miss
// (gardener v1.139.4 still writes both; the Secret has a removal TODO).
//
//   - both absent → (nil, nil): provisioning shoot, or a wildcard-cert seed
//   - forbidden → (nil, nil) + warn: the credential lacks configmaps get;
//     retrying cannot fix that, the operator bundle may still verify
//   - any other read error → error: a guessed "no CA" would be cached for
//     minutes, a failed resolution is retried in seconds
func (c *RealClient) caBundle(ctx context.Context, namespace, shootName string) ([]byte, error) {
	key := types.NamespacedName{Namespace: namespace, Name: shootName + caClusterSuffix}

	configMap := &corev1.ConfigMap{}
	configMapErr := c.client.Get(ctx, key, configMap)
	if configMapErr == nil {
		pem, ok := c.usableCA(ctx, []byte(configMap.Data[caBundleKey]), key, "config map")
		if ok {
			return pem, nil
		}
	}

	secret := &corev1.Secret{}
	secretErr := c.client.Get(ctx, key, secret)
	if secretErr == nil {
		pem, ok := c.usableCA(ctx, secret.Data[caBundleKey], key, "secret")
		if ok {
			if configMapErr != nil && !apierrors.IsNotFound(configMapErr) {
				c.logger.WarnContext(ctx, "shoot cluster CA config map unreadable, using the deprecated secret",
					"namespace", key.Namespace, "name", key.Name, "error", configMapErr)
			}
			return pem, nil
		}
	}

	var forbidden error
	for _, err := range []error{configMapErr, secretErr} {
		switch {
		case err == nil || apierrors.IsNotFound(err):
		case apierrors.IsForbidden(err):
			forbidden = err
		default:
			return nil, fmt.Errorf("read shoot cluster CA %s/%s: %w", key.Namespace, key.Name, err)
		}
	}
	if forbidden != nil {
		c.logger.WarnContext(ctx, "shoot cluster CA not readable, grant get on configmaps to the Gardener credential",
			"namespace", key.Namespace, "name", key.Name, "error", forbidden)
		return nil, nil
	}
	c.logger.DebugContext(ctx, "shoot publishes no cluster CA",
		"namespace", key.Namespace, "name", key.Name)
	return nil, nil
}

// usableCA reports whether pem holds a certificate; empty is "not published
// here", non-empty garbage is warned about.
func (c *RealClient) usableCA(ctx context.Context, pem []byte, key types.NamespacedName, source string) ([]byte, bool) {
	if len(pem) == 0 {
		return nil, false
	}
	if !x509.NewCertPool().AppendCertsFromPEM(pem) {
		c.logger.WarnContext(ctx, "shoot cluster CA holds no certificate",
			"namespace", key.Namespace, "name", key.Name, "source", source)
		return nil, false
	}
	return pem, true
}

// NoopClient is the zero-config implementation used when no Gardener
// kubeconfig is wired in (mock mode, local dev without Gardener).
type NoopClient struct{}

// Monitoring always returns ErrNotFound.
func (NoopClient) Monitoring(context.Context, uuid.UUID) (*MonitoringInfo, error) {
	return nil, ErrNotFound
}
