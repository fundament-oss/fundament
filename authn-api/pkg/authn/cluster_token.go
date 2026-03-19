package authn

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	authenticationv1 "k8s.io/api/authentication/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/fundament-oss/fundament/authn-api/pkg/authnhttp"
	db "github.com/fundament-oss/fundament/authn-api/pkg/db/gen"
)

const (
	// saTokenExpiry is the expiration time for service account tokens (24 hours).
	saTokenExpiry int64 = 86400

	// adminKubeconfigExpiry is the expiration time for the admin kubeconfig used
	// to issue SA tokens (10 minutes).
	adminKubeconfigExpiry int64 = 600

	// fundamentSystemNamespace is the namespace where service accounts are created.
	fundamentSystemNamespace = "fundament-system"
)

// AdminKubeconfig holds the result of an AdminKubeconfigRequest.
type AdminKubeconfig struct {
	Kubeconfig []byte
	ExpiresAt  time.Time
}

// GardenerClient is the interface authn-api uses to request admin kubeconfigs.
type GardenerClient interface {
	RequestAdminKubeconfig(ctx context.Context, clusterID uuid.UUID, expirationSeconds int64) (*AdminKubeconfig, error)
}

// HandleClusterToken issues a service account token for a cluster.
func (s *AuthnServer) HandleClusterToken(w http.ResponseWriter, r *http.Request, clusterID openapi_types.UUID) {
	claims, err := s.validator.Validate(r.Header)
	if err != nil {
		s.writeErrorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if s.gardenerClient == nil {
		s.writeErrorJSON(w, http.StatusServiceUnavailable, "cluster token endpoint not available")
		return
	}

	clusterUUID := clusterID
	ctx := r.Context()

	cluster, err := s.queries.ClusterGetForToken(ctx, db.ClusterGetForTokenParams{ClusterID: clusterUUID})
	if err != nil {
		s.logger.Debug("cluster not found", "cluster_id", clusterUUID, "error", err)
		s.writeErrorJSON(w, http.StatusNotFound, "cluster not found")
		return
	}

	if !cluster.ShootStatus.Valid || cluster.ShootStatus.String != "ready" || !cluster.ShootApiServerUrl.Valid {
		s.writeErrorJSON(w, http.StatusServiceUnavailable, "cluster not ready")
		return
	}

	accessLevel, err := s.queries.ResolveUserAccess(ctx, db.ResolveUserAccessParams{UserID: claims.UserID(), ClusterID: clusterUUID})
	if err != nil {
		s.logger.Error("failed to resolve user access", "error", err, "user_id", claims.UserID(), "cluster_id", clusterUUID)
		s.writeErrorJSON(w, http.StatusInternalServerError, "internal error")
		return
	}

	if accessLevel == "none" {
		s.writeErrorJSON(w, http.StatusForbidden, "no access to this cluster")
		return
	}

	token, expiresAt, err := s.requestSAToken(ctx, clusterUUID, claims.UserID())
	if err != nil {
		s.logger.Error("failed to request SA token", "error", err, "cluster_id", clusterUUID, "user_id", claims.UserID())
		s.writeErrorJSON(w, http.StatusServiceUnavailable, "sync pending, try again shortly")
		return
	}

	s.logger.Info("cluster token issued",
		"cluster_id", clusterUUID,
		"user_id", claims.UserID(),
		"access_level", accessLevel,
	)

	if err := s.writeJSON(w, http.StatusOK, authnhttp.ClusterTokenResponse{
		Token:     token,
		ExpiresAt: expiresAt,
	}); err != nil {
		s.logger.Error("failed to write JSON response", "error", err)
	}
}

// requestSAToken requests an admin kubeconfig from Gardener, then uses it to
// create a TokenRequest for the user's service account on the shoot cluster.
func (s *AuthnServer) requestSAToken(ctx context.Context, clusterID, userID uuid.UUID) (string, time.Time, error) {
	adminKC, err := s.gardenerClient.RequestAdminKubeconfig(ctx, clusterID, adminKubeconfigExpiry)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("request admin kubeconfig: %w", err)
	}

	shootClient, err := shootClientFromKubeconfig(adminKC.Kubeconfig)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create shoot client: %w", err)
	}

	saName := fmt.Sprintf("fundament-%s", userID)
	expSeconds := saTokenExpiry

	tokenReq := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: &expSeconds,
		},
	}

	result, err := shootClient.CoreV1().ServiceAccounts(fundamentSystemNamespace).CreateToken(ctx, saName, tokenReq, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", time.Time{}, fmt.Errorf("service account %s not found (sync pending)", saName)
		}
		return "", time.Time{}, fmt.Errorf("create token for SA %s: %w", saName, err)
	}

	return result.Status.Token, result.Status.ExpirationTimestamp.Time, nil
}

// shootClientFromKubeconfig creates a kubernetes clientset from raw kubeconfig bytes.
func shootClientFromKubeconfig(kubeconfig []byte) (*kubernetes.Clientset, error) {
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}

	cs, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create clientset: %w", err)
	}

	return cs, nil
}
