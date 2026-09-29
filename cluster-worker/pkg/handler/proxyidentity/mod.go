// Package proxyidentity provisions kube-api-proxy's own identity on every
// ready shoot. kube-api-proxy serves a user's namespace listing as that user's
// ServiceAccount plus one extra group, so its ServiceAccount may impersonate
// ServiceAccounts in fundament-system and exactly that group; the group grants
// get/list/watch on namespaces and nothing else.
package proxyidentity

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	"github.com/fundament-oss/fundament/common/shootidentity"
)

const (
	// ImpersonatorName names the Role, ClusterRole and their bindings that
	// grant the proxy ServiceAccount its impersonation rights.
	ImpersonatorName = "fundament:kube-api-proxy-impersonator"

	labelComponent = "fundament.io/component"
	componentValue = "kube-api-proxy"
)

// ReadyClusterLister lists ready clusters; *db.Queries satisfies it.
type ReadyClusterLister interface {
	ClusterListReady(ctx context.Context) ([]uuid.UUID, error)
}

// Handler provisions the proxy identity on cluster-ready and re-asserts it on
// every reconcile tick.
type Handler struct {
	queries ReadyClusterLister
	shoot   shoot.ShootAccess
	logger  *slog.Logger
}

func New(pool *pgxpool.Pool, shootAccess shoot.ShootAccess, logger *slog.Logger) *Handler {
	return &Handler{
		queries: db.New(pool),
		shoot:   shootAccess,
		logger:  logger.With("handler", "proxyidentity"),
	}
}

// Sync handles the cluster-ready outbox event: the row id is the cluster id.
func (h *Handler) Sync(ctx context.Context, id uuid.UUID, sc handler.SyncContext) error {
	switch sc.EntityType {
	case handler.EntityCluster:
		return h.Provision(ctx, id)
	default:
		return fmt.Errorf("unexpected entity type %s for proxyidentity handler", sc.EntityType)
	}
}

// Reconcile re-asserts the identity on every ready cluster, isolating
// per-cluster failures.
func (h *Handler) Reconcile(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil //nolint:nilerr // graceful shutdown
	}

	clusterIDs, err := h.queries.ClusterListReady(ctx)
	if err != nil {
		return fmt.Errorf("list ready clusters: %w", err)
	}

	var errs []error
	for _, clusterID := range clusterIDs {
		if ctx.Err() != nil {
			return nil //nolint:nilerr // graceful shutdown
		}
		err := h.Provision(ctx, clusterID)
		if err != nil {
			h.logger.Error("failed to provision kube-api-proxy identity", "cluster_id", clusterID, "error", err)
			errs = append(errs, err)
		}
	}

	err = errors.Join(errs...)
	if err != nil {
		return fmt.Errorf("proxy identity reconcile: %w", err)
	}
	return nil
}

// Provision converges one shoot to the desired identity. Every step is
// idempotent.
func (h *Handler) Provision(ctx context.Context, clusterID uuid.UUID) error {
	ns := shootidentity.Namespace
	labels := map[string]string{labelComponent: componentValue}
	proxySA := []rbacv1.Subject{{Kind: "ServiceAccount", Name: shootidentity.ProxyServiceAccount, Namespace: ns}}

	steps := []struct {
		desc string
		run  func() error
	}{
		{"namespace", func() error { return h.shoot.EnsureNamespace(ctx, clusterID, ns) }},
		{"service account", func() error {
			return h.shoot.EnsureServiceAccount(ctx, clusterID, ns, shootidentity.ProxyServiceAccount, labels, nil)
		}},
		{"impersonate-serviceaccounts role", func() error {
			return h.shoot.EnsureRole(ctx, clusterID, ns, ImpersonatorName, ImpersonateServiceAccountsRules(), labels)
		}},
		{"impersonate-serviceaccounts binding", func() error {
			return h.shoot.EnsureRoleBinding(ctx, clusterID, ns, ImpersonatorName,
				rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: ImpersonatorName}, proxySA, labels)
		}},
		{"impersonate-group cluster role", func() error {
			return h.shoot.EnsureClusterRole(ctx, clusterID, ImpersonatorName, ImpersonateGroupRules(), labels)
		}},
		{"impersonate-group binding", func() error {
			return h.shoot.EnsureClusterRoleBindingSubjects(ctx, clusterID, ImpersonatorName,
				rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: ImpersonatorName}, proxySA, labels)
		}},
		{"namespace-lister cluster role", func() error {
			return h.shoot.EnsureClusterRole(ctx, clusterID, shootidentity.NamespaceListerClusterRole, NamespaceListerRules(), labels)
		}},
		{"namespace-lister binding", func() error {
			return h.shoot.EnsureClusterRoleBindingSubjects(ctx, clusterID, shootidentity.NamespaceListerClusterRole,
				rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: shootidentity.NamespaceListerClusterRole},
				[]rbacv1.Subject{{Kind: rbacv1.GroupKind, APIGroup: rbacv1.GroupName, Name: shootidentity.NamespaceListerGroup}}, labels)
		}},
	}
	for _, s := range steps {
		err := s.run()
		if err != nil {
			return fmt.Errorf("ensure kube-api-proxy %s on cluster %s: %w", s.desc, clusterID, err)
		}
	}
	h.logger.Debug("kube-api-proxy identity provisioned", "cluster_id", clusterID)
	return nil
}

// ImpersonateServiceAccountsRules lets the proxy act as any ServiceAccount in
// the namespace the Role lives in (fundament-system): the per-user SAs.
func ImpersonateServiceAccountsRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"serviceaccounts"}, Verbs: []string{"impersonate"}}}
}

// ImpersonateGroupRules lets the proxy add exactly the namespace-lister group.
func ImpersonateGroupRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{{
		APIGroups:     []string{""},
		Resources:     []string{"groups"},
		Verbs:         []string{"impersonate"},
		ResourceNames: []string{shootidentity.NamespaceListerGroup},
	}}
}

// NamespaceListerRules is everything the namespace-lister group may do.
func NamespaceListerRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"namespaces"}, Verbs: []string{"get", "list", "watch"}}}
}
