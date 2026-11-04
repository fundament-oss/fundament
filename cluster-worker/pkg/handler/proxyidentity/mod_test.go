package proxyidentity

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	"github.com/fundament-oss/fundament/common/shootidentity"
)

type fakeClusters []uuid.UUID

func (f fakeClusters) ClusterListAccessible(context.Context) ([]db.ClusterListAccessibleRow, error) {
	rows := make([]db.ClusterListAccessibleRow, 0, len(f))
	for _, id := range f {
		rows = append(rows, db.ClusterListAccessibleRow{ID: id, Ready: true})
	}
	return rows, nil
}

func newTestHandler(t *testing.T, clusters ...uuid.UUID) (*Handler, *shoot.MockShootAccess) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mock := shoot.NewMockShootAccess(logger)
	return &Handler{queries: fakeClusters(clusters), shoot: mock, logger: logger}, mock
}

func TestSync_ProvisionsExactIdentity(t *testing.T) {
	t.Parallel()
	clusterID := uuid.New()
	h, mock := newTestHandler(t)

	require.NoError(t, h.Sync(context.Background(), clusterID, handler.SyncContext{EntityType: handler.EntityCluster}))

	ns := shootidentity.Namespace
	proxySA := []rbacv1.Subject{{Kind: "ServiceAccount", Name: shootidentity.ProxyServiceAccount, Namespace: ns}}

	_, ok := mock.ServiceAccounts[clusterID][ns][shootidentity.ProxyServiceAccount]
	assert.True(t, ok, "proxy service account")

	role := mock.Roles[clusterID][ns+"/"+ImpersonatorName]
	assert.Equal(t, []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"serviceaccounts"}, Verbs: []string{"impersonate"}}}, role.Rules)
	rb := mock.GetRoleBinding(clusterID, ns, ImpersonatorName)
	require.NotNil(t, rb)
	assert.Equal(t, rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: ImpersonatorName}, rb.RoleRef)
	assert.Equal(t, proxySA, rb.Subjects)

	groupRole := mock.ClusterRoles[clusterID][ImpersonatorName]
	assert.Equal(t, []rbacv1.PolicyRule{{
		APIGroups: []string{""}, Resources: []string{"groups"}, Verbs: []string{"impersonate"},
		ResourceNames: []string{"fundament:namespace-listers"},
	}}, groupRole.Rules)
	groupCRB := mock.ClusterRoleBindings[clusterID][ImpersonatorName]
	assert.Equal(t, proxySA, groupCRB.Subjects)
	assert.Equal(t, "ClusterRole", groupCRB.RoleRef.Kind)

	lister := mock.ClusterRoles[clusterID]["fundament:namespace-lister"]
	assert.Equal(t, []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"namespaces"}, Verbs: []string{"get", "list", "watch"}}}, lister.Rules)
	listerCRB := mock.ClusterRoleBindings[clusterID]["fundament:namespace-lister"]
	assert.Equal(t, []rbacv1.Subject{{Kind: "Group", APIGroup: rbacv1.GroupName, Name: "fundament:namespace-listers"}}, listerCRB.Subjects)

	scReader, ok := mock.ClusterRoles[clusterID]["fundament:storageclass-reader"]
	require.True(t, ok, "storageclass-reader cluster role")
	assert.Equal(t, []rbacv1.PolicyRule{{APIGroups: []string{"storage.k8s.io"}, Resources: []string{"storageclasses"}, Verbs: []string{"get", "list", "watch"}}}, scReader.Rules)
	scReaderCRB, ok := mock.ClusterRoleBindings[clusterID]["fundament:storageclass-reader"]
	require.True(t, ok, "storageclass-reader binding")
	assert.Equal(t, rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "fundament:storageclass-reader"}, scReaderCRB.RoleRef)
	assert.Equal(t, []rbacv1.Subject{{Kind: "Group", APIGroup: rbacv1.GroupName, Name: "system:serviceaccounts:fundament-system"}}, scReaderCRB.Subjects)

	// None of these objects carry the user-id label, so usersync's and the
	// project-RBAC converger's orphan cleanup never touch them.
	_, labelled := rb.Labels[shoot.LabelUserID]
	assert.False(t, labelled)
}

func TestReconcile_ProvisionsEveryReadyClusterAndJoinsErrors(t *testing.T) {
	t.Parallel()
	a, b := uuid.New(), uuid.New()
	h, mock := newTestHandler(t, a, b)

	require.NoError(t, h.Reconcile(context.Background()))
	assert.NotNil(t, mock.GetRoleBinding(a, shootidentity.Namespace, ImpersonatorName))
	assert.NotNil(t, mock.GetRoleBinding(b, shootidentity.Namespace, ImpersonatorName))

	mock.EnsureRoleError = errors.New("forbidden")
	require.ErrorContains(t, h.Reconcile(context.Background()), "forbidden")
}

func TestSync_RejectsOtherEntities(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler(t)
	require.Error(t, h.Sync(context.Background(), uuid.New(), handler.SyncContext{EntityType: handler.EntityNamespace}))
}
