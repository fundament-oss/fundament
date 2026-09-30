package main

import (
	"context"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// userRoles are the rights project members get on databases. Fundament binds
// project admins to ClusterRole/admin and viewers to view in their namespaces
// (FUN-7); these roles aggregate into both.
//
// They are the user half of "no update or delete on a Cluster". The plugin's
// own rbac only caps its console pages; the console's generated views and
// kubectl act with the user's own rights. The chart's
// rbac.aggregateClusterRoles would give admin every verb, delete included, so
// it stays off. The data itself is not protected: admin still deletes the
// PVCs and pods the operator creates (see README, Scope).
func userRoles() []*rbacv1.ClusterRole {
	clusters := func(verbs ...string) []rbacv1.PolicyRule {
		return []rbacv1.PolicyRule{{
			APIGroups: []string{"postgresql.cnpg.io"},
			Resources: []string{"clusters"},
			Verbs:     verbs,
		}}
	}
	return []*rbacv1.ClusterRole{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "fundament-cloudnativepg-admin",
				Labels: map[string]string{
					"rbac.authorization.k8s.io/aggregate-to-admin": "true",
					"app.kubernetes.io/managed-by":                 "fundament-cloudnativepg",
				},
			},
			Rules: clusters("get", "list", "watch", "create"),
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "fundament-cloudnativepg-view",
				Labels: map[string]string{
					"rbac.authorization.k8s.io/aggregate-to-view": "true",
					"app.kubernetes.io/managed-by":                "fundament-cloudnativepg",
				},
			},
			Rules: clusters("get", "list", "watch"),
		},
	}
}

// applyUserRoles creates or updates userRoles. Uninstalling the plugin leaves
// them, like the operator and the databases.
func applyUserRoles(ctx context.Context, kube client.Client) error {
	for _, want := range userRoles() {
		role := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: want.Name}}
		if _, err := controllerutil.CreateOrUpdate(ctx, kube, role, func() error {
			role.Labels = want.Labels
			role.Rules = want.Rules
			return nil
		}); err != nil {
			return fmt.Errorf("apply ClusterRole %s: %w", want.Name, err)
		}
	}
	return nil
}
