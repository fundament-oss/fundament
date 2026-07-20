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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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
//
// groups must carry the SA's virtual groups: SAR evaluates exactly the User +
// Groups it is handed and does not derive an SA's groups from the username, so
// a grant bound to one of those groups is invisible without them.
func (c *Client) SubjectAccessReview(ctx context.Context, clusterID string, user string, groups []string, attrs authorizationv1.ResourceAttributes) (bool, error) {
	adminKC, err := c.GetAdminKubeconfig(ctx, clusterID, 0)
	if err != nil {
		return false, fmt.Errorf("get admin kubeconfig: %w", err)
	}

	shootClient, err := clientsetFromKubeconfig(adminKC.Kubeconfig)
	if err != nil {
		return false, fmt.Errorf("create shoot client: %w", err)
	}

	return SubjectAccessReviewWithClientset(ctx, shootClient, user, groups, attrs)
}

// SubjectAccessReviewWithClientset posts a SubjectAccessReview to the cluster
// behind cs and returns the API server's Allowed decision. The SAR body and
// virtual-group handling live here so the Gardener path (admin kubeconfig) and
// the sandbox path (file kubeconfig) stay identical.
func SubjectAccessReviewWithClientset(ctx context.Context, cs kubernetes.Interface, user string, groups []string, attrs authorizationv1.ResourceAttributes) (bool, error) {
	sar := &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			User:               user,
			Groups:             groups,
			ResourceAttributes: &attrs,
		},
	}

	result, err := cs.AuthorizationV1().SubjectAccessReviews().Create(ctx, sar, metav1.CreateOptions{})
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

// ResolvePluginSA fetches the target cluster's admin kubeconfig and resolves the
// plugin SA token via ResolvePluginSAFromRESTConfig. installationID is the
// PluginInstallation CR's UID and installationName its metadata.name — both are
// carried in the PluginToken claim.
func (c *Client) ResolvePluginSA(ctx context.Context, clusterID, installationID, installationName string) (*PluginSAToken, error) {
	adminKC, err := c.GetAdminKubeconfig(ctx, clusterID, 0)
	if err != nil {
		return nil, fmt.Errorf("get admin kubeconfig: %w", err)
	}

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(adminKC.Kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}

	return ResolvePluginSAFromRESTConfig(ctx, restConfig, installationID, installationName)
}

// ResolvePluginSAFromRESTConfig resolves the PluginInstallation CR on the cluster
// identified by restConfig and mints a short-lived TokenRequest for its SA,
// returning the token plus the CR's pinned definitionHash. A missing CR or SA is
// wrapped in ErrSyncPending so callers can retry.
//
// Shared by the Gardener path (admin kubeconfig) and the sandbox path (file
// kubeconfig) so the lookup and naming live in one place.
func ResolvePluginSAFromRESTConfig(ctx context.Context, restConfig *rest.Config, installationID, installationName string) (*PluginSAToken, error) {
	// Read CRs via the dynamic client so we don't have to import
	// plugin-controller/pkg/api/v1 into kube-api-proxy.
	dyn, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}
	kube, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create shoot client: %w", err)
	}
	return resolvePluginSA(ctx, dyn, kube, installationID, installationName)
}

// resolvePluginSA is the client-injected core of ResolvePluginSAFromRESTConfig,
// split out so it can be exercised with fake clients.
//
// The CR is addressed by installationName (metadata.name), because the kube API
// can't Get by UID. installationID (the CR UID) stays authoritative: the
// resolved CR's UID is verified against it, so a stale token for a
// deleted-and-recreated install is rejected. The SA/namespace are named
// `plugin-{cr.Name}` (see plugin-controller/pkg/controller/resources.go).
func resolvePluginSA(ctx context.Context, dyn dynamic.Interface, kube kubernetes.Interface, installationID, installationName string) (*PluginSAToken, error) {
	if installationName == "" {
		return nil, fmt.Errorf("token missing installation_name for installation %q", installationID)
	}

	cr, err := dyn.Resource(pluginInstallationGVR).Get(ctx, installationName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("PluginInstallation %q not found: %w", installationName, ErrSyncPending)
		}
		return nil, fmt.Errorf("get PluginInstallation %q: %w", installationName, err)
	}
	if got := string(cr.GetUID()); got != installationID {
		return nil, fmt.Errorf("PluginInstallation %q uid %q does not match token installation_id %q: %w", installationName, got, installationID, ErrSyncPending)
	}

	// spec.definitionRef.definitionHash is optional — reconciler.go accepts
	// empty when AllowUnpinnedHash is set. Empty here just means the audit
	// event carries an empty hash.
	hash, _, err := unstructured.NestedString(cr.Object, "spec", "definitionRef", "definitionHash")
	if err != nil {
		return nil, fmt.Errorf("read spec.definitionRef.definitionHash from %q: %w", installationName, err)
	}

	saName := "plugin-" + installationName
	saNamespace := "plugin-" + installationName
	expSeconds := saTokenExpiry
	tokenReq := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: &expSeconds,
		},
	}
	result, err := kube.CoreV1().ServiceAccounts(saNamespace).CreateToken(
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
