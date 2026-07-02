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
	"k8s.io/apimachinery/pkg/api/resource"
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
	// newClient builds a Kubernetes client for a shoot. It exists as a field so
	// tests can inject a fake clientset; in production it is nil and
	// clientForCluster falls back to the real AdminKubeconfigRequest path.
	newClient func(ctx context.Context, clusterID uuid.UUID) (kubernetes.Interface, error)
}

// NewRealShootAccess creates a ShootAccess backed by real Gardener AdminKubeconfigRequest calls.
func NewRealShootAccess(gardenerClient gardener.Client, logger *slog.Logger) *RealShootAccess {
	return &RealShootAccess{
		gardener: gardenerClient,
		logger:   logger.With("component", "shoot-access"),
	}
}

func (r *RealShootAccess) clientForCluster(ctx context.Context, clusterID uuid.UUID) (kubernetes.Interface, error) {
	if r.newClient != nil {
		return r.newClient(ctx, clusterID)
	}

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

// resourceClient is the subset of a typed client-go resource interface that
// the ensure/delete helpers need; every typed client satisfies it.
type resourceClient[T any] interface {
	Create(ctx context.Context, obj *T, opts metav1.CreateOptions) (*T, error)
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*T, error)
	Update(ctx context.Context, obj *T, opts metav1.UpdateOptions) (*T, error)
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
}

// ensureResource creates desired and, when it already exists, reconciles it:
// merge mutates the fetched object toward the desired state and returns true
// when the change requires a delete+recreate (immutable field). A nil merge
// treats any existing object as up to date.
func ensureResource[T any](ctx context.Context, c resourceClient[T], name, desc string, desired *T, merge func(existing *T) (recreate bool)) error {
	_, err := c.Create(ctx, desired, metav1.CreateOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create %s: %w", desc, err)
	}
	if merge == nil {
		return nil
	}
	existing, err := c.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get existing %s: %w", desc, err)
	}
	if merge(existing) {
		if err := c.Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete existing %s before recreate: %w", desc, err)
		}
		if _, err := c.Create(ctx, desired, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("recreate %s: %w", desc, err)
		}
		return nil
	}
	if _, err := c.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update %s: %w", desc, err)
	}
	return nil
}

// deleteResource deletes by name, treating NotFound as success.
func deleteResource[T any](ctx context.Context, c resourceClient[T], name, desc string) error {
	err := c.Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete %s: %w", desc, err)
	}
	return nil
}

// mergeMeta copies labels and annotations onto an existing object's metadata,
// initializing nil maps; keys not listed are left untouched.
func mergeMeta(meta *metav1.ObjectMeta, labels, annotations map[string]string) {
	if meta.Labels == nil {
		meta.Labels = make(map[string]string)
	}
	if meta.Annotations == nil {
		meta.Annotations = make(map[string]string)
	}
	maps.Copy(meta.Labels, labels)
	maps.Copy(meta.Annotations, annotations)
}

func (r *RealShootAccess) EnsureNamespace(ctx context.Context, clusterID uuid.UUID, name string) error {
	cs, err := r.clientForCluster(ctx, clusterID)
	if err != nil {
		return err
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	return ensureResource(ctx, cs.CoreV1().Namespaces(), name, "namespace "+name, ns, nil)
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

	return deleteResource(ctx, cs.CoreV1().Namespaces(), name, "namespace "+name)
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

	return ensureResource(ctx, cs.CoreV1().ServiceAccounts(namespace), name, fmt.Sprintf("SA %s/%s", namespace, name), sa,
		func(existing *corev1.ServiceAccount) bool {
			mergeMeta(&existing.ObjectMeta, labels, annotations)
			return false
		})
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

	return ensureResource(ctx, cs.RbacV1().ClusterRoleBindings(), name, "CRB "+name, crb,
		func(existing *rbacv1.ClusterRoleBinding) bool {
			if ClusterRoleBindingNeedsRecreate(existing, crb) {
				return true
			}
			mergeMeta(&existing.ObjectMeta, labels, annotations)
			existing.Subjects = crb.Subjects
			return false
		})
}

func (r *RealShootAccess) DeleteServiceAccount(ctx context.Context, clusterID uuid.UUID, namespace, name string) error {
	cs, err := r.clientForCluster(ctx, clusterID)
	if err != nil {
		return err
	}

	return deleteResource(ctx, cs.CoreV1().ServiceAccounts(namespace), name, fmt.Sprintf("SA %s/%s", namespace, name))
}

func (r *RealShootAccess) DeleteClusterRoleBinding(ctx context.Context, clusterID uuid.UUID, name string) error {
	cs, err := r.clientForCluster(ctx, clusterID)
	if err != nil {
		return err
	}

	return deleteResource(ctx, cs.RbacV1().ClusterRoleBindings(), name, "CRB "+name)
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

func (r *RealShootAccess) EnsureLimitRange(ctx context.Context, clusterID uuid.UUID, namespace string, defaults LimitDefaults, labels map[string]string) error {
	cs, err := r.clientForCluster(ctx, clusterID)
	if err != nil {
		return err
	}

	lr := &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LimitRangeName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: limitRangeSpec(defaults),
	}

	return ensureResource(ctx, cs.CoreV1().LimitRanges(namespace), LimitRangeName, fmt.Sprintf("LimitRange %s/%s", namespace, LimitRangeName), lr,
		func(existing *corev1.LimitRange) bool {
			mergeMeta(&existing.ObjectMeta, labels, nil)
			existing.Spec = lr.Spec
			return false
		})
}

func (r *RealShootAccess) DeleteLimitRange(ctx context.Context, clusterID uuid.UUID, namespace string) error {
	cs, err := r.clientForCluster(ctx, clusterID)
	if err != nil {
		return err
	}

	return deleteResource(ctx, cs.CoreV1().LimitRanges(namespace), LimitRangeName, fmt.Sprintf("LimitRange %s/%s", namespace, LimitRangeName))
}

// limitRangeSpec converts the defaults to a single-Container LimitRangeSpec.
// Only set fields are populated: `default` (the limit ceiling) from the limit
// values, `defaultRequest` from the request values; CPU as millicores (500m),
// memory as mebibytes (512Mi).
func limitRangeSpec(defaults LimitDefaults) corev1.LimitRangeSpec {
	defaultLimits := corev1.ResourceList{}
	defaultRequests := corev1.ResourceList{}
	if defaults.CPULimitMilli != nil {
		defaultLimits[corev1.ResourceCPU] = *resource.NewMilliQuantity(int64(*defaults.CPULimitMilli), resource.DecimalSI)
	}
	if defaults.MemoryLimitMi != nil {
		defaultLimits[corev1.ResourceMemory] = *resource.NewQuantity(int64(*defaults.MemoryLimitMi)<<20, resource.BinarySI)
	}
	if defaults.CPURequestMilli != nil {
		defaultRequests[corev1.ResourceCPU] = *resource.NewMilliQuantity(int64(*defaults.CPURequestMilli), resource.DecimalSI)
	}
	if defaults.MemoryRequestMi != nil {
		defaultRequests[corev1.ResourceMemory] = *resource.NewQuantity(int64(*defaults.MemoryRequestMi)<<20, resource.BinarySI)
	}

	item := corev1.LimitRangeItem{Type: corev1.LimitTypeContainer}
	if len(defaultLimits) > 0 {
		item.Default = defaultLimits
	}
	if len(defaultRequests) > 0 {
		item.DefaultRequest = defaultRequests
	}
	return corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{item}}
}

// ClusterRoleBindingNeedsRecreate returns true if the RoleRef has changed (immutable field).
func ClusterRoleBindingNeedsRecreate(existing, desired *rbacv1.ClusterRoleBinding) bool {
	return existing.RoleRef != desired.RoleRef
}

var _ ShootAccess = (*RealShootAccess)(nil)
