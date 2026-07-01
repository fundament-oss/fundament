package proxy

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kubereq"
)

// UserAccessChecker issues a SubjectAccessReview against the target cluster for
// the acting USER's per-user SA identity (fundament-{userID}). This is FUN-17's
// "user half".
type UserAccessChecker interface {
	Check(ctx context.Context, clusterID string, attrs *kubereq.Attributes, userID string) (bool, error)
}

// PluginSA carries the plugin installation's ServiceAccount token (obtained via
// a short-lived TokenRequest) and the definition hash pinned on the CR at
// request time.
type PluginSA struct {
	Token                string
	PinnedDefinitionHash string
}

// PluginSAResolver resolves (cluster, installation) to the plugin SA token. The
// cluster's RBAC on that SA — the ClusterRole materialised by plugin-controller
// from the pinned definition — is FUN-17's "plugin half".
type PluginSAResolver interface {
	Resolve(ctx context.Context, clusterID, installationID string) (PluginSA, error)
}

// ClusterViewChecker is the OpenFGA can_view check (already available in the
// Server as the authz client; adapt to the existing method).
type ClusterViewChecker interface {
	CanViewCluster(ctx context.Context, userID, clusterID uuid.UUID) (bool, error)
}

// pluginGateway implements the PluginToken path of the kube-api-proxy gateway.
type pluginGateway struct {
	logger      *slog.Logger
	jwtSecret   []byte
	userSAR     UserAccessChecker
	pluginSA    PluginSAResolver
	canView     ClusterViewChecker
	kubeHandler http.Handler

	// lastForwarded is a test hook; nil in production.
	lastForwarded func() *http.Request
}

// serve runs the PluginToken gates in order and forwards on success.
func (g *pluginGateway) serve(w http.ResponseWriter, r *http.Request, pathClusterID string) {
	bearer := bearerToken(r)
	claims, err := auth.ParsePluginToken(bearer, g.jwtSecret)
	if err != nil {
		g.logger.Debug("plugin token invalid", "err", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// 1. Cluster binding.
	if claims.ClusterID != pathClusterID {
		http.Error(w, "cluster_id mismatch", http.StatusForbidden)
		return
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		http.Error(w, "bad subject", http.StatusUnauthorized)
		return
	}
	clusterUUID, err := uuid.Parse(pathClusterID)
	if err != nil {
		http.Error(w, "bad cluster id", http.StatusBadRequest)
		return
	}

	// 2. OpenFGA can_view on (user, cluster).
	ok, err := g.canView.CanViewCluster(r.Context(), userID, clusterUUID)
	if err != nil {
		g.logger.Error("can_view check failed", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "permission denied", http.StatusForbidden)
		return
	}

	// 3. Parse the kube request.
	attrs, err := kubereq.Parse(r)
	if err != nil {
		http.Error(w, "unparseable kube request", http.StatusBadRequest)
		return
	}

	// 4. SubjectAccessReview against the USER (user half).
	allowed, err := g.userSAR.Check(r.Context(), pathClusterID, &attrs, claims.Subject)
	if err != nil {
		g.logger.Error("user SAR failed", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if !allowed {
		g.audit(claims, &attrs, "", "denied:user-sar")
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// 5. Resolve the plugin SA token (plugin half is the cluster's RBAC on it).
	sa, err := g.pluginSA.Resolve(r.Context(), pathClusterID, claims.InstallationID)
	if err != nil {
		g.logger.Error("plugin SA resolve failed", "err", err)
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}

	// 6. Forward, injecting the PLUGIN SA token. The cluster's RBAC on that SA
	//    completes the intersection.
	g.audit(claims, &attrs, sa.PinnedDefinitionHash, "allowed")
	ctx := WithSAToken(r.Context(), sa.Token)
	ctx = WithUserID(ctx, userID)
	g.kubeHandler.ServeHTTP(w, r.WithContext(ctx))
}

func bearerToken(r *http.Request) string {
	const p = "Bearer "
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, p) {
		return h[len(p):]
	}
	return ""
}
