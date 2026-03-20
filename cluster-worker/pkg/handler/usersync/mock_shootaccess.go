package usersync

import (
	"context"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	rbacv1 "k8s.io/api/rbac/v1"
)

// MockShootAccess implements ShootAccess with in-memory state for testing and mock mode.
type MockShootAccess struct {
	mu     sync.RWMutex
	logger *slog.Logger

	// Per-cluster state: clusterID → namespace → SA name → resource metadata
	ServiceAccounts map[uuid.UUID]map[string]map[string]ResourceInfo
	// Per-cluster CRBs: clusterID → CRB name → resource metadata
	ClusterRoleBindings map[uuid.UUID]map[string]ResourceInfo
	// Namespaces: clusterID → namespace names
	Namespaces map[uuid.UUID]map[string]bool

	// Configurable errors for testing
	EnsureNamespaceError          error
	EnsureServiceAccountError     error
	EnsureClusterRoleBindingError error
	DeleteServiceAccountError     error
	DeleteClusterRoleBindingError error
	ListServiceAccountsError      error
	ListClusterRoleBindingsError  error
}

func NewMockShootAccess(logger *slog.Logger) *MockShootAccess {
	return &MockShootAccess{
		logger:              logger.With("component", "mock-shoot-access"),
		ServiceAccounts:     make(map[uuid.UUID]map[string]map[string]ResourceInfo),
		ClusterRoleBindings: make(map[uuid.UUID]map[string]ResourceInfo),
		Namespaces:          make(map[uuid.UUID]map[string]bool),
	}
}

func (m *MockShootAccess) EnsureNamespace(_ context.Context, clusterID uuid.UUID, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.EnsureNamespaceError != nil {
		return m.EnsureNamespaceError
	}

	if m.Namespaces[clusterID] == nil {
		m.Namespaces[clusterID] = make(map[string]bool)
	}
	m.Namespaces[clusterID][name] = true
	m.logger.Debug("MOCK: ensured namespace", "cluster_id", clusterID, "namespace", name)
	return nil
}

func (m *MockShootAccess) EnsureServiceAccount(_ context.Context, clusterID uuid.UUID, namespace, name string, labels, annotations map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.EnsureServiceAccountError != nil {
		return m.EnsureServiceAccountError
	}

	if m.ServiceAccounts[clusterID] == nil {
		m.ServiceAccounts[clusterID] = make(map[string]map[string]ResourceInfo)
	}
	if m.ServiceAccounts[clusterID][namespace] == nil {
		m.ServiceAccounts[clusterID][namespace] = make(map[string]ResourceInfo)
	}
	m.ServiceAccounts[clusterID][namespace][name] = ResourceInfo{
		Name:        name,
		Labels:      cloneStringMap(labels),
		Annotations: cloneStringMap(annotations),
	}
	m.logger.Debug("MOCK: ensured SA", "cluster_id", clusterID, "namespace", namespace, "name", name)
	return nil
}

func (m *MockShootAccess) EnsureClusterRoleBinding(_ context.Context, clusterID uuid.UUID, name, saNamespace, saName string, labels, annotations map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.EnsureClusterRoleBindingError != nil {
		return m.EnsureClusterRoleBindingError
	}

	if m.ClusterRoleBindings[clusterID] == nil {
		m.ClusterRoleBindings[clusterID] = make(map[string]ResourceInfo)
	}
	m.ClusterRoleBindings[clusterID][name] = ResourceInfo{
		Name:        name,
		Labels:      cloneStringMap(labels),
		Annotations: cloneStringMap(annotations),
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      saName,
			Namespace: saNamespace,
		}},
	}
	m.logger.Debug("MOCK: ensured CRB", "cluster_id", clusterID, "name", name)
	return nil
}

func (m *MockShootAccess) DeleteServiceAccount(_ context.Context, clusterID uuid.UUID, namespace, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.DeleteServiceAccountError != nil {
		return m.DeleteServiceAccountError
	}

	if m.ServiceAccounts[clusterID] != nil && m.ServiceAccounts[clusterID][namespace] != nil {
		delete(m.ServiceAccounts[clusterID][namespace], name)
	}
	m.logger.Debug("MOCK: deleted SA", "cluster_id", clusterID, "namespace", namespace, "name", name)
	return nil
}

func (m *MockShootAccess) DeleteClusterRoleBinding(_ context.Context, clusterID uuid.UUID, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.DeleteClusterRoleBindingError != nil {
		return m.DeleteClusterRoleBindingError
	}

	if m.ClusterRoleBindings[clusterID] != nil {
		delete(m.ClusterRoleBindings[clusterID], name)
	}
	m.logger.Debug("MOCK: deleted CRB", "cluster_id", clusterID, "name", name)
	return nil
}

func (m *MockShootAccess) ListServiceAccounts(_ context.Context, clusterID uuid.UUID, namespace string) ([]ResourceInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.ListServiceAccountsError != nil {
		return nil, m.ListServiceAccountsError
	}

	var result []ResourceInfo
	if m.ServiceAccounts[clusterID] != nil && m.ServiceAccounts[clusterID][namespace] != nil {
		for _, resource := range m.ServiceAccounts[clusterID][namespace] {
			result = append(result, resource)
		}
	}
	return result, nil
}

func (m *MockShootAccess) ListClusterRoleBindings(_ context.Context, clusterID uuid.UUID, _ string) ([]ResourceInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.ListClusterRoleBindingsError != nil {
		return nil, m.ListClusterRoleBindingsError
	}

	var result []ResourceInfo
	if m.ClusterRoleBindings[clusterID] != nil {
		for _, resource := range m.ClusterRoleBindings[clusterID] {
			result = append(result, resource)
		}
	}
	return result, nil
}

// HasSA checks if a ServiceAccount exists for a user on a cluster.
func (m *MockShootAccess) HasSA(clusterID, userID uuid.UUID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.ServiceAccounts[clusterID] == nil || m.ServiceAccounts[clusterID][FundamentNamespace] == nil {
		return false
	}
	_, ok := m.ServiceAccounts[clusterID][FundamentNamespace][SAName(userID)]
	return ok
}

// HasCRB checks if a ClusterRoleBinding exists for a user on a cluster.
func (m *MockShootAccess) HasCRB(clusterID, userID uuid.UUID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.ClusterRoleBindings[clusterID] == nil {
		return false
	}
	_, ok := m.ClusterRoleBindings[clusterID][CRBName(userID)]
	return ok
}

// Reset clears all state.
func (m *MockShootAccess) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ServiceAccounts = make(map[uuid.UUID]map[string]map[string]ResourceInfo)
	m.ClusterRoleBindings = make(map[uuid.UUID]map[string]ResourceInfo)
	m.Namespaces = make(map[uuid.UUID]map[string]bool)
}

var _ ShootAccess = (*MockShootAccess)(nil)
