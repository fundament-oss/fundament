package shoot

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"sync"

	"github.com/google/uuid"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/yaml"
)

// MockShootAccess implements ShootAccess with in-memory state for testing and mock mode.
type MockShootAccess struct {
	mu     sync.RWMutex
	logger *slog.Logger

	// Per-cluster state: clusterID -> namespace -> SA name -> resource metadata
	ServiceAccounts map[uuid.UUID]map[string]map[string]ResourceInfo
	// Per-cluster CRBs: clusterID -> CRB name -> resource metadata
	ClusterRoleBindings map[uuid.UUID]map[string]ResourceInfo
	// Namespaces: clusterID -> namespace name -> resource metadata (with labels)
	Namespaces map[uuid.UUID]map[string]ResourceInfo
	// LimitRanges: clusterID -> namespace name -> the managed fundament-defaults LimitRange
	LimitRanges map[uuid.UUID]map[string]MockLimitRange
	// CRDs: clusterID -> CRD name (metadata.name from the manifest) -> manifest bytes
	CRDs map[uuid.UUID]map[string][]byte
	// ClusterRoles: clusterID -> ClusterRole name -> rules + labels
	ClusterRoles map[uuid.UUID]map[string]MockClusterRole
	// Deployments: clusterID -> "namespace/name" -> the applied Deployment
	Deployments map[uuid.UUID]map[string]*appsv1.Deployment

	// Configurable errors for testing
	EnsureNamespaceError          error
	EnsureServiceAccountError     error
	EnsureClusterRoleBindingError error
	DeleteServiceAccountError     error
	DeleteClusterRoleBindingError error
	ListServiceAccountsError      error
	ListClusterRoleBindingsError  error
	GetNamespaceError             error
	CreateNamespaceError          error
	UpdateNamespaceLabelsError    error
	DeleteNamespaceError          error
	ListNamespacesError           error
	EnsureLimitRangeError         error
	DeleteLimitRangeError         error
	EnsureCRDError                error
	EnsureClusterRoleError        error
	EnsureDeploymentError         error
}

// MockClusterRole is the in-memory representation of an applied ClusterRole.
type MockClusterRole struct {
	Rules  []rbacv1.PolicyRule
	Labels map[string]string
}

// MockLimitRange is the in-memory representation of the managed LimitRange.
type MockLimitRange struct {
	Defaults LimitDefaults
	Labels   map[string]string
}

func NewMockShootAccess(logger *slog.Logger) *MockShootAccess {
	return &MockShootAccess{
		logger:              logger.With("component", "mock-shoot-access"),
		ServiceAccounts:     make(map[uuid.UUID]map[string]map[string]ResourceInfo),
		ClusterRoleBindings: make(map[uuid.UUID]map[string]ResourceInfo),
		Namespaces:          make(map[uuid.UUID]map[string]ResourceInfo),
		LimitRanges:         make(map[uuid.UUID]map[string]MockLimitRange),
		CRDs:                make(map[uuid.UUID]map[string][]byte),
		ClusterRoles:        make(map[uuid.UUID]map[string]MockClusterRole),
		Deployments:         make(map[uuid.UUID]map[string]*appsv1.Deployment),
	}
}

func (m *MockShootAccess) EnsureNamespace(_ context.Context, clusterID uuid.UUID, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.EnsureNamespaceError != nil {
		return m.EnsureNamespaceError
	}

	if m.Namespaces[clusterID] == nil {
		m.Namespaces[clusterID] = make(map[string]ResourceInfo)
	}
	if _, ok := m.Namespaces[clusterID][name]; !ok {
		m.Namespaces[clusterID][name] = ResourceInfo{Name: name}
	}
	m.logger.Debug("MOCK: ensured namespace", "cluster_id", clusterID, "namespace", name)
	return nil
}

func (m *MockShootAccess) GetNamespace(_ context.Context, clusterID uuid.UUID, name string) (*ResourceInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.GetNamespaceError != nil {
		return nil, m.GetNamespaceError
	}

	ns, ok := m.Namespaces[clusterID][name]
	if !ok {
		return nil, nil //nolint:nilnil // absence is signalled by a nil result, not an error
	}
	clone := ResourceInfo{Name: ns.Name, Labels: maps.Clone(ns.Labels), Annotations: maps.Clone(ns.Annotations)}
	return &clone, nil
}

func (m *MockShootAccess) CreateNamespace(_ context.Context, clusterID uuid.UUID, name string, labels map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.CreateNamespaceError != nil {
		return m.CreateNamespaceError
	}

	if m.Namespaces[clusterID] == nil {
		m.Namespaces[clusterID] = make(map[string]ResourceInfo)
	}
	if _, ok := m.Namespaces[clusterID][name]; ok {
		// Mirror the real client: Create on an existing name is a conflict, not a
		// no-op. The handler relies on this to detect a lost create race.
		return apierrors.NewAlreadyExists(corev1.Resource("namespaces"), name)
	}
	m.Namespaces[clusterID][name] = ResourceInfo{Name: name, Labels: maps.Clone(labels)}
	m.logger.Debug("MOCK: created namespace", "cluster_id", clusterID, "namespace", name)
	return nil
}

func (m *MockShootAccess) UpdateNamespaceLabels(_ context.Context, clusterID uuid.UUID, name string, labels map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.UpdateNamespaceLabelsError != nil {
		return m.UpdateNamespaceLabelsError
	}

	ns, ok := m.Namespaces[clusterID][name]
	if !ok {
		return nil
	}
	if ns.Labels == nil {
		ns.Labels = make(map[string]string)
	}
	maps.Copy(ns.Labels, labels)
	m.Namespaces[clusterID][name] = ns
	m.logger.Debug("MOCK: updated namespace labels", "cluster_id", clusterID, "namespace", name)
	return nil
}

func (m *MockShootAccess) DeleteNamespace(_ context.Context, clusterID uuid.UUID, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.DeleteNamespaceError != nil {
		return m.DeleteNamespaceError
	}

	if m.Namespaces[clusterID] != nil {
		delete(m.Namespaces[clusterID], name)
	}
	m.logger.Debug("MOCK: deleted namespace", "cluster_id", clusterID, "namespace", name)
	return nil
}

func (m *MockShootAccess) ListNamespaces(_ context.Context, clusterID uuid.UUID, labelKey string) ([]ResourceInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.ListNamespacesError != nil {
		return nil, m.ListNamespacesError
	}

	var result []ResourceInfo
	for _, ns := range m.Namespaces[clusterID] {
		if labelKey != "" {
			if _, ok := ns.Labels[labelKey]; !ok {
				continue
			}
		}
		result = append(result, ResourceInfo{Name: ns.Name, Labels: maps.Clone(ns.Labels), Annotations: maps.Clone(ns.Annotations)})
	}
	return result, nil
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
		Labels:      maps.Clone(labels),
		Annotations: maps.Clone(annotations),
	}
	m.logger.Debug("MOCK: ensured SA", "cluster_id", clusterID, "namespace", namespace, "name", name)
	return nil
}

func (m *MockShootAccess) EnsureClusterRoleBinding(_ context.Context, clusterID uuid.UUID, name, roleName, saNamespace, saName string, labels, annotations map[string]string) error {
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
		Labels:      maps.Clone(labels),
		Annotations: maps.Clone(annotations),
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     roleName,
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

func (m *MockShootAccess) EnsureCRD(_ context.Context, clusterID uuid.UUID, manifest []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.EnsureCRDError != nil {
		return m.EnsureCRDError
	}

	var meta struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
	}
	err := yaml.Unmarshal(manifest, &meta)
	if err != nil {
		return fmt.Errorf("unmarshal CRD manifest: %w", err)
	}

	if m.CRDs[clusterID] == nil {
		m.CRDs[clusterID] = make(map[string][]byte)
	}
	m.CRDs[clusterID][meta.Metadata.Name] = append([]byte(nil), manifest...)
	m.logger.Debug("MOCK: ensured CRD", "cluster_id", clusterID, "name", meta.Metadata.Name)
	return nil
}

func (m *MockShootAccess) EnsureClusterRole(_ context.Context, clusterID uuid.UUID, name string, rules []rbacv1.PolicyRule, labels map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.EnsureClusterRoleError != nil {
		return m.EnsureClusterRoleError
	}

	if m.ClusterRoles[clusterID] == nil {
		m.ClusterRoles[clusterID] = make(map[string]MockClusterRole)
	}
	m.ClusterRoles[clusterID][name] = MockClusterRole{
		Rules:  append([]rbacv1.PolicyRule(nil), rules...),
		Labels: maps.Clone(labels),
	}
	m.logger.Debug("MOCK: ensured ClusterRole", "cluster_id", clusterID, "name", name)
	return nil
}

func (m *MockShootAccess) EnsureDeployment(_ context.Context, clusterID uuid.UUID, deployment *appsv1.Deployment) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.EnsureDeploymentError != nil {
		return m.EnsureDeploymentError
	}

	if m.Deployments[clusterID] == nil {
		m.Deployments[clusterID] = make(map[string]*appsv1.Deployment)
	}
	m.Deployments[clusterID][deployment.Namespace+"/"+deployment.Name] = deployment.DeepCopy()
	m.logger.Debug("MOCK: ensured Deployment", "cluster_id", clusterID, "namespace", deployment.Namespace, "name", deployment.Name)
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

func (m *MockShootAccess) ListServiceAccounts(_ context.Context, clusterID uuid.UUID, namespace, labelKey string) ([]ResourceInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.ListServiceAccountsError != nil {
		return nil, m.ListServiceAccountsError
	}

	var result []ResourceInfo
	if m.ServiceAccounts[clusterID] != nil && m.ServiceAccounts[clusterID][namespace] != nil {
		for _, resource := range m.ServiceAccounts[clusterID][namespace] {
			if labelKey != "" {
				if _, ok := resource.Labels[labelKey]; !ok {
					continue
				}
			}
			result = append(result, resource)
		}
	}
	return result, nil
}

func (m *MockShootAccess) ListClusterRoleBindings(_ context.Context, clusterID uuid.UUID, labelKey string) ([]ResourceInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.ListClusterRoleBindingsError != nil {
		return nil, m.ListClusterRoleBindingsError
	}

	var result []ResourceInfo
	if m.ClusterRoleBindings[clusterID] != nil {
		for _, resource := range m.ClusterRoleBindings[clusterID] {
			if labelKey != "" {
				if _, ok := resource.Labels[labelKey]; !ok {
					continue
				}
			}
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

func (m *MockShootAccess) EnsureLimitRange(_ context.Context, clusterID uuid.UUID, namespace string, defaults LimitDefaults, labels map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.EnsureLimitRangeError != nil {
		return m.EnsureLimitRangeError
	}

	if m.LimitRanges[clusterID] == nil {
		m.LimitRanges[clusterID] = make(map[string]MockLimitRange)
	}
	m.LimitRanges[clusterID][namespace] = MockLimitRange{Defaults: defaults, Labels: maps.Clone(labels)}
	m.logger.Debug("MOCK: ensured limit range", "cluster_id", clusterID, "namespace", namespace)
	return nil
}

func (m *MockShootAccess) DeleteLimitRange(_ context.Context, clusterID uuid.UUID, namespace string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.DeleteLimitRangeError != nil {
		return m.DeleteLimitRangeError
	}

	delete(m.LimitRanges[clusterID], namespace)
	m.logger.Debug("MOCK: deleted limit range", "cluster_id", clusterID, "namespace", namespace)
	return nil
}

// GetLimitRange returns the managed LimitRange for a namespace, or nil if absent.
func (m *MockShootAccess) GetLimitRange(clusterID uuid.UUID, namespace string) *MockLimitRange {
	m.mu.RLock()
	defer m.mu.RUnlock()

	lr, ok := m.LimitRanges[clusterID][namespace]
	if !ok {
		return nil
	}
	clone := MockLimitRange{Defaults: lr.Defaults, Labels: maps.Clone(lr.Labels)}
	return &clone
}

// Reset clears all state.
func (m *MockShootAccess) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ServiceAccounts = make(map[uuid.UUID]map[string]map[string]ResourceInfo)
	m.ClusterRoleBindings = make(map[uuid.UUID]map[string]ResourceInfo)
	m.Namespaces = make(map[uuid.UUID]map[string]ResourceInfo)
	m.LimitRanges = make(map[uuid.UUID]map[string]MockLimitRange)
	m.CRDs = make(map[uuid.UUID]map[string][]byte)
	m.ClusterRoles = make(map[uuid.UUID]map[string]MockClusterRole)
	m.Deployments = make(map[uuid.UUID]map[string]*appsv1.Deployment)
}

var _ ShootAccess = (*MockShootAccess)(nil)
