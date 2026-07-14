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
	authorizationv1 "k8s.io/api/authorization/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
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

// SubjectAccessReview issues a SelfSubject-less SAR against the target
// cluster's authorization API with a caller-provided Spec.User and
// ResourceAttributes. Returns the API server's Status.Allowed decision.
//
// Used by the plugin gateway's "user half": the acting user's per-user SA
// identity (system:serviceaccount:fundament-system:fundament-{userID}, synced
// onto tenant clusters by cluster-worker) is checked against the concrete
// verb/resource the plugin is trying to reach. Combined with the plugin
// SA's own RBAC (the "plugin half") this produces the FUN-17 intersection.
func (c *Client) SubjectAccessReview(ctx context.Context, clusterID string, user string, attrs authorizationv1.ResourceAttributes) (bool, error) {
	adminKC, err := c.GetAdminKubeconfig(ctx, clusterID, 0)
	if err != nil {
		return false, fmt.Errorf("get admin kubeconfig: %w", err)
	}

	shootClient, err := clientsetFromKubeconfig(adminKC.Kubeconfig)
	if err != nil {
		return false, fmt.Errorf("create shoot client: %w", err)
	}

	sar := &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			User:               user,
			ResourceAttributes: &attrs,
		},
	}

	result, err := shootClient.AuthorizationV1().SubjectAccessReviews().Create(ctx, sar, metav1.CreateOptions{})
	if err != nil {
		return false, fmt.Errorf("create SubjectAccessReview: %w", err)
	}

	return result.Status.Allowed, nil
}

// pluginInstallationGVR is the GroupVersionResource for the plugin-controller
// PluginInstallation CRD. Duplicated as a literal here so kube-api-proxy stays
// free of a direct dependency on plugin-controller's types package.
var pluginInstallationGVR = schema.GroupVersionResource{
	Group:    "plugins.fundament.io",
	Version:  "v1",
	Resource: "plugininstallations",
}

// PluginSAToken bundles a plugin SA token with the pinned definition hash
// that was on the installation CR when the token was issued. The gateway
// audits both together so a forensic query can join by hash.
type PluginSAToken struct {
	Token                string
	ExpiresAt            time.Time
	PinnedDefinitionHash string
}

// ResolvePluginSA reads the PluginInstallation CR (cluster-scoped) from the
// target cluster and mints a short-lived TokenRequest for its SA. Assumes
// plugin-controller runs on every tenant cluster, so the SA + namespace live
// on the same cluster the plugin JS is trying to reach.
//
// The SA name is `plugin-{installationName}` in namespace `plugin-{installationName}`
// (see plugin-controller/pkg/controller/resources.go).
func (c *Client) ResolvePluginSA(ctx context.Context, clusterID, installationName string) (*PluginSAToken, error) {
	adminKC, err := c.GetAdminKubeconfig(ctx, clusterID, 0)
	if err != nil {
		return nil, fmt.Errorf("get admin kubeconfig: %w", err)
	}

	// Read the CR via the dynamic client so we don't have to import
	// plugin-controller/pkg/api/v1 into kube-api-proxy.
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(adminKC.Kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}

	dyn, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}

	cr, err := dyn.Resource(pluginInstallationGVR).Get(ctx, installationName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("PluginInstallation %q not found: %w", installationName, ErrSyncPending)
		}
		return nil, fmt.Errorf("get PluginInstallation %q: %w", installationName, err)
	}

	// spec.definitionRef.definitionHash is optional — reconciler.go accepts
	// empty when AllowUnpinnedHash is set. Empty here just means the audit
	// event carries an empty hash.
	hash, _, err := unstructuredNestedString(cr.Object, "spec", "definitionRef", "definitionHash")
	if err != nil {
		return nil, fmt.Errorf("read spec.definitionRef.definitionHash from %q: %w", installationName, err)
	}

	shootClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create shoot client: %w", err)
	}

	saName := "plugin-" + installationName
	saNamespace := "plugin-" + installationName
	expSeconds := saTokenExpiry
	tokenReq := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: &expSeconds,
		},
	}
	result, err := shootClient.CoreV1().ServiceAccounts(saNamespace).CreateToken(
		ctx, saName, tokenReq, metav1.CreateOptions{},
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("plugin SA %s/%s not found: %w", saNamespace, saName, ErrSyncPending)
		}
		return nil, fmt.Errorf("create token for plugin SA %s/%s: %w", saNamespace, saName, err)
	}

	return &PluginSAToken{
		Token:                result.Status.Token,
		ExpiresAt:            result.Status.ExpirationTimestamp.Time,
		PinnedDefinitionHash: hash,
	}, nil
}

// unstructuredNestedString walks a nested map[string]any (as returned by the
// dynamic client) and returns the string at the final key, or "" if any hop
// is missing. Returns an error only if a hop exists but has the wrong type —
// callers get to distinguish "field not set" from "schema drift".
func unstructuredNestedString(obj map[string]any, path ...string) (string, bool, error) {
	var current any = obj
	for i, key := range path {
		m, ok := current.(map[string]any)
		if !ok {
			return "", false, fmt.Errorf("path[%d]=%q: parent is not an object", i, key)
		}
		v, present := m[key]
		if !present {
			return "", false, nil
		}
		current = v
	}
	if current == nil {
		return "", false, nil
	}
	s, ok := current.(string)
	if !ok {
		return "", false, fmt.Errorf("value at %v is not a string (got %T)", path, current)
	}
	return s, true, nil
}
