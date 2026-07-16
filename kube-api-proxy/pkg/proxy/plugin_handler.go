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
}

// serve runs the PluginToken gates in order and forwards on success.
//
// Every gate outcome is audited so the forensic log has a line per
// PluginToken request. The one exception is a token that fails to parse —
// without verified claims there is no user to attribute, and the raw bearer
// must not be logged.
func (g *pluginGateway) serve(w http.ResponseWriter, r *http.Request, clusterID uuid.UUID) {
	bearer := bearerToken(r)
	claims, err := auth.ParsePluginToken(bearer, g.jwtSecret)
	if err != nil {
		g.logger.Debug("plugin token invalid", "err", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// 1. Cluster binding. Parse the claim as a UUID so canonicalisation
	//    differences (case, formatting) can't produce a spurious 403.
	claimCluster, err := uuid.Parse(claims.ClusterID)
	if err != nil || claimCluster != clusterID {
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
	ok, err := g.canView.CanViewCluster(r.Context(), userID, clusterID)
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
	// TODO(FUN-17): non-resource discovery endpoints (/api, /apis,
	// /apis/{g}/{v}, /openapi/...) don't fit the resource grammar and fail here
	// with 400, so client-go/kubectl-based plugins can't complete discovery.
	// This is intentionally fail-closed for now; a discovery carve-out would
	// SAR-check them as nonResourceURLs rather than rejecting outright.
	attrs, err := kubereq.Parse(r)
	if err != nil {
		g.audit(claims, nil, "", "error:unparseable-request")
		http.Error(w, "unparseable kube request", http.StatusBadRequest)
		return
	}

	// 4. SubjectAccessReview against the USER (user half).
	allowed, err := g.userSAR.Check(r.Context(), clusterID.String(), &attrs, claims.Subject)
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
	sa, err := g.pluginSA.Resolve(r.Context(), clusterID.String(), claims.InstallationID)
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
