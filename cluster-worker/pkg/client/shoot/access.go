package shoot

import (
	"context"

	"github.com/google/uuid"
	rbacv1 "k8s.io/api/rbac/v1"
)

const (
	// FundamentNamespace is where all fundament ServiceAccounts are created.
	FundamentNamespace = "fundament-system"

	// LabelUserID is the label key for fundament-managed resources.
	LabelUserID = "fundament.io/user-id"

	// AnnotationUserName is the annotation key for user email.
	AnnotationUserName = "fundament.io/user-name"

	// LimitRangeName is the name of the managed LimitRange that materializes
	// the merged organization/project per-container resource defaults inside a
	// project namespace.
	LimitRangeName = "fundament-defaults"
)

// LimitDefaults are the effective per-container resource defaults applied as
// the managed LimitRange. Nil fields are omitted from the object. CPU values
// are millicores, memory values mebibytes (matching the tenant.*_limits
// column units).
type LimitDefaults struct {
	CPURequestMilli *int32
	CPULimitMilli   *int32
	MemoryRequestMi *int32
	MemoryLimitMi   *int32
}

// SAName returns the ServiceAccount name for a user.
func SAName(userID uuid.UUID) string {
	return "fundament-" + userID.String()
}

// CRBName returns the ClusterRoleBinding name for an admin user.
func CRBName(userID uuid.UUID) string {
	return "fundament:admin:" + userID.String()
}

// ResourceInfo contains the metadata needed by reconciliation.
type ResourceInfo struct {
	Name        string
	Labels      map[string]string
	Annotations map[string]string
	RoleRef     rbacv1.RoleRef
	Subjects    []rbacv1.Subject
}

// ShootAccess provides operations on shoot clusters for user access management.
type ShootAccess interface {
	// EnsureNamespace creates the namespace if it doesn't exist.
	EnsureNamespace(ctx context.Context, clusterID uuid.UUID, name string) error

	// GetNamespace returns the namespace's metadata, or nil if it does not exist.
	GetNamespace(ctx context.Context, clusterID uuid.UUID, name string) (*ResourceInfo, error)

	// CreateNamespace creates a namespace with the given labels.
	CreateNamespace(ctx context.Context, clusterID uuid.UUID, name string, labels map[string]string) error

	// UpdateNamespaceLabels merges the given labels onto an existing namespace,
	// preserving labels not in the provided set (e.g. operator-added labels).
	UpdateNamespaceLabels(ctx context.Context, clusterID uuid.UUID, name string, labels map[string]string) error

	// DeleteNamespace deletes a namespace (no-op if absent).
	DeleteNamespace(ctx context.Context, clusterID uuid.UUID, name string) error

	// ListNamespaces lists namespaces filtered by label key existence.
	ListNamespaces(ctx context.Context, clusterID uuid.UUID, labelKey string) ([]ResourceInfo, error)

	// EnsureLimitRange creates or updates the managed fundament-defaults
	// LimitRange in a namespace to match the given defaults.
	EnsureLimitRange(ctx context.Context, clusterID uuid.UUID, namespace string, defaults LimitDefaults, labels map[string]string) error

	// DeleteLimitRange deletes the managed fundament-defaults LimitRange from
	// a namespace (no-op if absent).
	DeleteLimitRange(ctx context.Context, clusterID uuid.UUID, namespace string) error

	// EnsureServiceAccount creates or updates a ServiceAccount.
	EnsureServiceAccount(ctx context.Context, clusterID uuid.UUID, namespace, name string, labels, annotations map[string]string) error

	// EnsureClusterRoleBinding creates or updates a ClusterRoleBinding binding to cluster-admin.
	EnsureClusterRoleBinding(ctx context.Context, clusterID uuid.UUID, name, saNamespace, saName string, labels, annotations map[string]string) error

	// DeleteServiceAccount deletes a ServiceAccount (no-op if absent).
	DeleteServiceAccount(ctx context.Context, clusterID uuid.UUID, namespace, name string) error

	// DeleteClusterRoleBinding deletes a ClusterRoleBinding (no-op if absent).
	DeleteClusterRoleBinding(ctx context.Context, clusterID uuid.UUID, name string) error

	// ListServiceAccounts lists ServiceAccounts in a namespace filtered by label key existence.
	ListServiceAccounts(ctx context.Context, clusterID uuid.UUID, namespace, labelKey string) ([]ResourceInfo, error)

	// ListClusterRoleBindings lists ClusterRoleBindings filtered by label key existence.
	ListClusterRoleBindings(ctx context.Context, clusterID uuid.UUID, labelKey string) ([]ResourceInfo, error)
}
