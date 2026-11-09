package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/google/uuid"
	"github.com/jellydator/ttlcache/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/gardener"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kube"
)

// noAccessMessage refuses a user without a ServiceAccount on the cluster
// (docs/user/clusters.md, "What the kubeconfig grants").
const noAccessMessage = "only organization admins and members of a project on this cluster can access it"

// onCluster reports whether the user holds a role that gives them a
// ServiceAccount on the cluster: organization admin, or member of one of its
// projects (cluster-worker's UserListForCluster).
func (v *visibilityResolver) onCluster(ctx context.Context, userID, clusterID uuid.UUID) (bool, error) {
	key := visibilityKey{userID: userID, clusterID: clusterID}
	if item := v.roles.Get(key); item != nil {
		return item.Value(), nil
	}
	vis, err := v.resolve(ctx, userID, clusterID)
	if err != nil {
		return false, err
	}
	hasRole := vis.admin
	if !hasRole && len(vis.projectIDs) > 0 {
		// The user's viewable projects span every cluster; keep this cluster's.
		onCluster, err := v.authz.ListObjects(ctx, authz.Cluster(clusterID), authz.Parent(), authz.ObjectTypeProject)
		if err != nil {
			return false, fmt.Errorf("list projects of the cluster: %w", err)
		}
		hasRole = slices.ContainsFunc(onCluster, func(id string) bool {
			_, found := slices.BinarySearch(vis.projectIDs, id)
			return found
		})
	}
	v.roles.Set(key, hasRole, ttlcache.DefaultTTL)
	return hasRole, nil
}

// userTokenContext adds the user's ServiceAccount token for the cluster to
// ctx, with what to do when the cluster rejects it.
func (s *Server) userTokenContext(ctx context.Context, userID, clusterID uuid.UUID) (context.Context, error) {
	saToken, err := s.tokenCache.GetToken(ctx, userID, clusterID.String())
	if err != nil {
		return ctx, err //nolint:wrapcheck // passed to writeUserTokenError, which reads the cause
	}
	ctx = WithSAToken(ctx, saToken)
	return kube.WithTokenRejected(ctx, func() {
		s.tokenCache.ForgetToken(userID, clusterID.String(), saToken)
	}), nil
}

// writeUserTokenError answers a request whose user has no ServiceAccount
// token. A missing ServiceAccount is still being set up only for a user with
// a role on the cluster; anyone else is refused.
func (s *Server) writeUserTokenError(w http.ResponseWriter, r *http.Request, userID, clusterID uuid.UUID, err error) {
	ctx := r.Context()
	if errors.Is(err, gardener.ErrSyncPending) && s.namespaces != nil {
		hasRole, roleErr := s.namespaces.visibility.onCluster(ctx, userID, clusterID)
		if roleErr == nil && !hasRole {
			s.logger.InfoContext(ctx, "refused: no role on the cluster", "cluster", clusterID)
			kube.WriteStatus(w, http.StatusForbidden, metav1.StatusReasonForbidden, noAccessMessage)
			return
		}
		if roleErr != nil {
			s.logger.ErrorContext(ctx, "check role on the cluster", "error", roleErr)
		}
	}
	s.logger.ErrorContext(ctx, "failed to get SA token", "error", err)
	kube.WriteStepError(w, kube.StepUserCredentials, err)
}

// retryUserRequest answers r with a new token after the cluster rejected the
// previous one, as when the user's ServiceAccount was deleted or created
// again. Only a request without a body can be sent again.
func (s *Server) retryUserRequest(w http.ResponseWriter, r *http.Request, userID, clusterID uuid.UUID) {
	ctx, err := s.userTokenContext(r.Context(), userID, clusterID)
	if err != nil {
		s.writeUserTokenError(w, r, userID, clusterID, err)
		return
	}
	// ContentLength 0 is when the reverse proxy sends no body (HTTP/2 never
	// sets Body to http.NoBody); a chunked body has -1.
	if r.ContentLength != 0 {
		kube.WriteStatus(w, http.StatusServiceUnavailable, metav1.StatusReasonServiceUnavailable,
			"your credentials for this cluster were renewed while the request was on its way; send it again")
		return
	}
	ctx = context.WithValue(ctx, kube.ClusterIDContextKey{}, clusterID.String())
	s.kubeHandler.ServeHTTP(w, r.WithContext(ctx))
}
