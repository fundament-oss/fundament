package proxy

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kubereq"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/pluginsa"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/useraccess"
)

// ClusterViewChecker is the OpenFGA can_view check (already available in the
// Server as the authz client; adapt to the existing method).
type ClusterViewChecker func(ctx context.Context, userID, clusterID uuid.UUID) (bool, error)

// pluginGateway implements the PluginToken path of the kube-api-proxy gateway.
type pluginGateway struct {
	logger      *slog.Logger
	jwtSecret   []byte
	userSAR     useraccess.Checker
	pluginSA    pluginsa.Resolver
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

	// 1. Cluster binding. Compare the claim and the path as parsed UUIDs so
	//    canonicalisation differences (case, formatting) can't produce a
	//    spurious mismatch.
	clusterUUID, err := uuid.Parse(pathClusterID)
	if err != nil {
		g.audit(claims, nil, "", "error:bad-cluster-id")
		http.Error(w, "bad cluster id", http.StatusBadRequest)
		return
	}
	claimCluster, err := uuid.Parse(claims.ClusterID)
	if err != nil || claimCluster != clusterUUID {
		g.audit(claims, nil, "", "denied:cluster-mismatch")
		http.Error(w, "cluster_id mismatch", http.StatusForbidden)
		return
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		g.audit(claims, nil, "", "error:bad-subject")
		http.Error(w, "bad subject", http.StatusUnauthorized)
		return
	}

	// 2. OpenFGA can_view on (user, cluster).
	ok, err := g.canView(r.Context(), userID, clusterUUID)
	if err != nil {
		g.logger.Error("can_view check failed", "err", err)
		g.audit(claims, nil, "", "error:can-view")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if !ok {
		g.audit(claims, nil, "", "denied:can-view")
		http.Error(w, "permission denied", http.StatusForbidden)
		return
	}

	// 3. Parse the kube request.
	attrs, err := kubereq.Parse(r)
	if err != nil {
		g.audit(claims, nil, "", "error:unparseable-request")
		http.Error(w, "unparseable kube request", http.StatusBadRequest)
		return
	}

	// 4. SubjectAccessReview against the USER (user half).
	allowed, err := g.userSAR.Check(r.Context(), pathClusterID, &attrs, claims.Subject)
	if err != nil {
		g.logger.Error("user SAR failed", "err", err)
		g.audit(claims, &attrs, "", "error:user-sar")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if !allowed {
		g.audit(claims, &attrs, "", "denied:user-sar")
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// 5. Resolve the plugin SA token (plugin half is the cluster's RBAC on it).
	sa, err := g.pluginSA.Resolve(r.Context(), pathClusterID, claims.InstallationID, claims.InstallationName)
	if err != nil {
		g.logger.Error("plugin SA resolve failed", "err", err)
		g.audit(claims, &attrs, "", "error:plugin-sa")
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
