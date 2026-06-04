package shoot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
)

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

func (r *RealShootAccess) GetNamespace(ctx context.Context, clusterID uuid.UUID, name string) (*ResourceInfo, error) {
	cs, err := r.clientForCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	ns, err := cs.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil //nolint:nilnil // absence is signalled by a nil result, not an error
	}
	if err != nil {
		return nil, fmt.Errorf("get namespace %s: %w", name, err)
	}
	return &ResourceInfo{
		Name:        ns.Name,
		Labels:      maps.Clone(ns.Labels),
		Annotations: maps.Clone(ns.Annotations),
	}, nil
}

func (r *RealShootAccess) CreateNamespace(ctx context.Context, clusterID uuid.UUID, name string, labels map[string]string) error {
	cs, err := r.clientForCluster(ctx, clusterID)
	if err != nil {
		return err
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
	// Do NOT swallow AlreadyExists here (unlike EnsureNamespace): the handler only
	// calls Create after confirming the name is absent, so a conflict means another
	// actor won a race for that name. Surfacing it lets the row retry and re-run the
	// ownership/label check rather than silently "adopting" a namespace that may not
	// carry our fundament.io/namespace-id label.
	if _, err := cs.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create namespace %s: %w", name, err)
	}
	return nil
}

func (r *RealShootAccess) UpdateNamespaceLabels(ctx context.Context, clusterID uuid.UUID, name string, labels map[string]string) error {
	cs, err := r.clientForCluster(ctx, clusterID)
	if err != nil {
		return err
	}

	// Merge the labels with a JSON merge patch rather than a read-modify-write
	// Update: it is a single atomic request (no lost-update race under concurrent
	// reconciles), and only the listed keys are set, so operator-added labels are
	// left untouched.
	patch, err := json.Marshal(map[string]any{
		"metadata": map[string]any{"labels": labels},
	})
	if err != nil {
		return fmt.Errorf("marshal namespace %s label patch: %w", name, err)
	}
	if _, err := cs.CoreV1().Namespaces().Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("patch namespace %s labels: %w", name, err)
	}
	return nil
}

func (r *RealShootAccess) DeleteNamespace(ctx context.Context, clusterID uuid.UUID, name string) error {
	cs, err := r.clientForCluster(ctx, clusterID)
	if err != nil {
		return err
	}

	err = cs.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete namespace %s: %w", name, err)
	}
	return nil
}

func (r *RealShootAccess) ListNamespaces(ctx context.Context, clusterID uuid.UUID, labelKey string) ([]ResourceInfo, error) {
	cs, err := r.clientForCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	list, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{LabelSelector: labelKey})
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}

	result := make([]ResourceInfo, len(list.Items))
	for i := range list.Items {
		result[i] = ResourceInfo{
			Name:        list.Items[i].Name,
			Labels:      maps.Clone(list.Items[i].Labels),
			Annotations: maps.Clone(list.Items[i].Annotations),
		}
	}
	return result, nil
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
		maps.Copy(existing.Labels, labels)
		maps.Copy(existing.Annotations, annotations)
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

		if ClusterRoleBindingNeedsRecreate(existing, crb) {
			if err := cs.RbacV1().ClusterRoleBindings().Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("delete existing CRB %s before recreate: %w", name, err)
			}
			if _, err := cs.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("recreate CRB %s: %w", name, err)
			}
			return nil
		}

		maps.Copy(existing.Labels, labels)
		maps.Copy(existing.Annotations, annotations)
		existing.Subjects = crb.Subjects
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

func (r *RealShootAccess) ListServiceAccounts(ctx context.Context, clusterID uuid.UUID, namespace, labelKey string) ([]ResourceInfo, error) {
	cs, err := r.clientForCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	list, err := cs.CoreV1().ServiceAccounts(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelKey})
	if err != nil {
		return nil, fmt.Errorf("list SAs in %s: %w", namespace, err)
	}

	result := make([]ResourceInfo, len(list.Items))
	for i := range list.Items {
		result[i] = ResourceInfo{
			Name:        list.Items[i].Name,
			Labels:      maps.Clone(list.Items[i].Labels),
			Annotations: maps.Clone(list.Items[i].Annotations),
		}
	}
	return result, nil
}

func (r *RealShootAccess) ListClusterRoleBindings(ctx context.Context, clusterID uuid.UUID, labelKey string) ([]ResourceInfo, error) {
	cs, err := r.clientForCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	list, err := cs.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{LabelSelector: labelKey})
	if err != nil {
		return nil, fmt.Errorf("list CRBs: %w", err)
	}

	result := make([]ResourceInfo, len(list.Items))
	for i := range list.Items {
		result[i] = ResourceInfo{
			Name:        list.Items[i].Name,
			Labels:      maps.Clone(list.Items[i].Labels),
			Annotations: maps.Clone(list.Items[i].Annotations),
			RoleRef:     list.Items[i].RoleRef,
			Subjects:    append([]rbacv1.Subject(nil), list.Items[i].Subjects...),
		}
	}
	return result, nil
}

// ClusterRoleBindingNeedsRecreate returns true if the RoleRef has changed (immutable field).
func ClusterRoleBindingNeedsRecreate(existing, desired *rbacv1.ClusterRoleBinding) bool {
	return existing.RoleRef != desired.RoleRef
}

var _ ShootAccess = (*RealShootAccess)(nil)
