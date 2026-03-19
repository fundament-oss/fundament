package usersync

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
)

const (
	// Namespace where all fundament ServiceAccounts are created.
	FundamentNamespace = "fundament-system"

	// Label keys for fundament-managed resources.
	LabelUserID   = "fundament.io/user-id"
	LabelUserName = "fundament.io/user-name"
)

// SAName returns the ServiceAccount name for a user.
func SAName(userID uuid.UUID) string {
	return "fundament-" + userID.String()
}

// CRBName returns the ClusterRoleBinding name for an admin user.
func CRBName(userID uuid.UUID) string {
	return "fundament:admin:" + userID.String()
}

// ResourceInfo contains the name and labels of a k8s resource.
type ResourceInfo struct {
	Name   string
	Labels map[string]string
}

// ShootAccess provides operations on shoot clusters for user access management.
type ShootAccess interface {
	// EnsureNamespace creates the namespace if it doesn't exist.
	EnsureNamespace(ctx context.Context, clusterID uuid.UUID, name string) error

	// EnsureServiceAccount creates or updates a ServiceAccount.
	EnsureServiceAccount(ctx context.Context, clusterID uuid.UUID, namespace, name string, labels, annotations map[string]string) error

	// EnsureClusterRoleBinding creates or updates a ClusterRoleBinding binding to cluster-admin.
	EnsureClusterRoleBinding(ctx context.Context, clusterID uuid.UUID, name, saNamespace, saName string, labels, annotations map[string]string) error

	// DeleteServiceAccount deletes a ServiceAccount (no-op if absent).
	DeleteServiceAccount(ctx context.Context, clusterID uuid.UUID, namespace, name string) error

	// DeleteClusterRoleBinding deletes a ClusterRoleBinding (no-op if absent).
	DeleteClusterRoleBinding(ctx context.Context, clusterID uuid.UUID, name string) error

	// ListServiceAccounts lists all ServiceAccounts in a namespace.
	ListServiceAccounts(ctx context.Context, clusterID uuid.UUID, namespace string) ([]ResourceInfo, error)

	// ListClusterRoleBindings lists ClusterRoleBindings matching the given label selector.
	ListClusterRoleBindings(ctx context.Context, clusterID uuid.UUID, labelSelector string) ([]ResourceInfo, error)
}

// RealShootAccess implements ShootAccess using AdminKubeconfigRequest to access shoot clusters.
type RealShootAccess struct {
	gardener gardener.Client
	logger   *slog.Logger
}

// NewRealShootAccess creates a ShootAccess backed by real Gardener AdminKubeconfigRequest calls.
func NewRealShootAccess(gardenerClient gardener.Client, logger *slog.Logger) *RealShootAccess {
	return &RealShootAccess{
		gardener: gardenerClient,
		logger:   logger.With("component", "shoot-access"),
	}
}

func (r *RealShootAccess) clientForCluster(ctx context.Context, clusterID uuid.UUID) (*kubernetes.Clientset, error) {
	adminKC, err := r.gardener.RequestAdminKubeconfig(ctx, clusterID, 600)
	if err != nil {
		return nil, fmt.Errorf("request admin kubeconfig: %w", err)
	}

	cfg, err := clientcmd.RESTConfigFromKubeConfig(adminKC.Kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}

	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create clientset: %w", err)
	}

	return cs, nil
}

func (r *RealShootAccess) EnsureNamespace(ctx context.Context, clusterID uuid.UUID, name string) error {
	cs, err := r.clientForCluster(ctx, clusterID)
	if err != nil {
		return err
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	_, err = cs.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create namespace %s: %w", name, err)
	}
	return nil
}

func (r *RealShootAccess) EnsureServiceAccount(ctx context.Context, clusterID uuid.UUID, namespace, name string, labels, annotations map[string]string) error {
	cs, err := r.clientForCluster(ctx, clusterID)
	if err != nil {
		return err
	}

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
	}

	_, err = cs.CoreV1().ServiceAccounts(namespace).Create(ctx, sa, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		existing, getErr := cs.CoreV1().ServiceAccounts(namespace).Get(ctx, name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("get existing SA %s/%s: %w", namespace, name, getErr)
		}
		existing.Labels = labels
		existing.Annotations = annotations
		_, err = cs.CoreV1().ServiceAccounts(namespace).Update(ctx, existing, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("update SA %s/%s: %w", namespace, name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("create SA %s/%s: %w", namespace, name, err)
	}
	return nil
}

func (r *RealShootAccess) EnsureClusterRoleBinding(ctx context.Context, clusterID uuid.UUID, name, saNamespace, saName string, labels, annotations map[string]string) error {
	cs, err := r.clientForCluster(ctx, clusterID)
	if err != nil {
		return err
	}

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      labels,
			Annotations: annotations,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      saName,
				Namespace: saNamespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
	}

	_, err = cs.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		existing, getErr := cs.RbacV1().ClusterRoleBindings().Get(ctx, name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("get existing CRB %s: %w", name, getErr)
		}
		existing.Labels = labels
		existing.Annotations = annotations
		existing.Subjects = crb.Subjects
		existing.RoleRef = crb.RoleRef
		_, err = cs.RbacV1().ClusterRoleBindings().Update(ctx, existing, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("update CRB %s: %w", name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("create CRB %s: %w", name, err)
	}
	return nil
}

func (r *RealShootAccess) DeleteServiceAccount(ctx context.Context, clusterID uuid.UUID, namespace, name string) error {
	cs, err := r.clientForCluster(ctx, clusterID)
	if err != nil {
		return err
	}

	err = cs.CoreV1().ServiceAccounts(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete SA %s/%s: %w", namespace, name, err)
	}
	return nil
}

func (r *RealShootAccess) DeleteClusterRoleBinding(ctx context.Context, clusterID uuid.UUID, name string) error {
	cs, err := r.clientForCluster(ctx, clusterID)
	if err != nil {
		return err
	}

	err = cs.RbacV1().ClusterRoleBindings().Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete CRB %s: %w", name, err)
	}
	return nil
}

func (r *RealShootAccess) ListServiceAccounts(ctx context.Context, clusterID uuid.UUID, namespace string) ([]ResourceInfo, error) {
	cs, err := r.clientForCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	list, err := cs.CoreV1().ServiceAccounts(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list SAs in %s: %w", namespace, err)
	}

	result := make([]ResourceInfo, len(list.Items))
	for i := range list.Items {
		result[i] = ResourceInfo{Name: list.Items[i].Name, Labels: list.Items[i].Labels}
	}
	return result, nil
}

func (r *RealShootAccess) ListClusterRoleBindings(ctx context.Context, clusterID uuid.UUID, labelSelector string) ([]ResourceInfo, error) {
	cs, err := r.clientForCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	list, err := cs.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, fmt.Errorf("list CRBs: %w", err)
	}

	result := make([]ResourceInfo, len(list.Items))
	for i := range list.Items {
		result[i] = ResourceInfo{Name: list.Items[i].Name, Labels: list.Items[i].Labels}
	}
	return result, nil
}
