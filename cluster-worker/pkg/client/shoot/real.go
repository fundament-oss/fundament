package shoot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"time"

	"github.com/google/uuid"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
)

// RealShootAccess implements ShootAccess using AdminKubeconfigRequest to access shoot clusters.
type RealShootAccess struct {
	gardener gardener.Client
	logger   *slog.Logger
	// newClient builds a Kubernetes client for a shoot. NewRealShootAccess wires
	// it to the real AdminKubeconfigRequest path; it is a field so tests can
	// substitute a fake clientset instead.
	newClient func(ctx context.Context, clusterID uuid.UUID) (kubernetes.Interface, error)
	// newAPIExtClient is the apiextensions counterpart of newClient, used by
	// EnsureCRD.
	newAPIExtClient func(ctx context.Context, clusterID uuid.UUID) (apiextensionsclientset.Interface, error)
}

// NewRealShootAccess creates a ShootAccess backed by real Gardener
// AdminKubeconfigRequest calls. Construct through here: the client factories
// are wired in this function, so a zero-value RealShootAccess has none.
func NewRealShootAccess(gardenerClient gardener.Client, logger *slog.Logger) *RealShootAccess {
	r := &RealShootAccess{
		gardener: gardenerClient,
		logger:   logger.With("component", "shoot-access"),
	}

	r.newClient = func(ctx context.Context, clusterID uuid.UUID) (kubernetes.Interface, error) {
		cfg, err := r.restConfigForCluster(ctx, clusterID)
		if err != nil {
			return nil, err
		}
		cs, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("create clientset: %w", err)
		}
		return cs, nil
	}

	r.newAPIExtClient = func(ctx context.Context, clusterID uuid.UUID) (apiextensionsclientset.Interface, error) {
		cfg, err := r.restConfigForCluster(ctx, clusterID)
		if err != nil {
			return nil, err
		}
		cs, err := apiextensionsclientset.NewForConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("create apiextensions clientset: %w", err)
		}
		return cs, nil
	}

	return r
}

func (r *RealShootAccess) restConfigForCluster(ctx context.Context, clusterID uuid.UUID) (*rest.Config, error) {
	cache := clientCacheFrom(ctx)
	if cache != nil {
		if cfg, ok := cache.get(clusterID); ok {
			return cfg, nil
		}
	}

	adminKC, err := r.gardener.RequestAdminKubeconfig(ctx, clusterID, 600)
	if err != nil {
		return nil, fmt.Errorf("request admin kubeconfig: %w", err)
	}

	cfg, err := clientcmd.RESTConfigFromKubeConfig(adminKC.Kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}

	if cache != nil {
		cache.put(clusterID, cfg)
	}

	return cfg, nil
}

// resourceClient is the subset of a typed client-go resource interface that
// the ensure/delete helpers need; every typed client satisfies it.
type resourceClient[T any] interface {
	Create(ctx context.Context, obj *T, opts metav1.CreateOptions) (*T, error)
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*T, error)
	Update(ctx context.Context, obj *T, opts metav1.UpdateOptions) (*T, error)
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
}

// mergeAction is what ensureResource should do with an object that already exists.
type mergeAction int

const (
	// mergeUpdate writes the merged object back.
	mergeUpdate mergeAction = iota
	// mergeRecreate replaces the object, because an immutable field changed.
	mergeRecreate
	// mergeSkip leaves the live object exactly as it is.
	mergeSkip
)

// ensureResource creates desired and, when it already exists, reconciles it:
// merge mutates the fetched object toward the desired state and returns what
// should happen to it. A nil merge treats any existing object as up to date.
func ensureResource[T any](ctx context.Context, c resourceClient[T], name, desc string, desired *T, merge func(existing *T) mergeAction) error {
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
	action := merge(existing)
	switch action {
	case mergeSkip:
		return nil
	case mergeRecreate:
		if err := deleteAndWait(ctx, c, name, desc); err != nil {
			return err
		}
		if _, err := c.Create(ctx, desired, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("recreate %s: %w", desc, err)
		}
		return nil
	case mergeUpdate:
		if _, err := c.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update %s: %w", desc, err)
		}
		return nil
	default:
		panic(fmt.Sprintf("unhandled merge action %d for %s", action, desc))
	}
}

// Bounds on the wait between a recreate's delete and its create.
const (
	recreateDeleteTimeout  = 30 * time.Second
	recreateDeletePollWait = 250 * time.Millisecond
)

// deleteAndWait deletes an object and blocks until it is really gone, so the
// create that follows cannot fail with AlreadyExists. Both halves matter:
// Delete without an explicit policy gets the resource's own default, and for
// Deployments that is foreground deletion, which keeps the object alive behind
// a foregroundDeletion finalizer until its ReplicaSets and Pods are collected.
// Background propagation drops the object immediately and cleans up dependents
// behind it; the poll then covers any other finalizer still holding it.
func deleteAndWait[T any](ctx context.Context, c resourceClient[T], name, desc string) error {
	policy := metav1.DeletePropagationBackground
	if err := c.Delete(ctx, name, metav1.DeleteOptions{PropagationPolicy: &policy}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete existing %s before recreate: %w", desc, err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, recreateDeleteTimeout)
	defer cancel()

	err := wait.PollUntilContextCancel(waitCtx, recreateDeletePollWait, true, func(ctx context.Context) (bool, error) {
		_, err := c.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, fmt.Errorf("get %s while waiting for deletion: %w", desc, err)
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("wait for %s to be deleted before recreate: %w", desc, err)
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
	cs, err := r.newClient(ctx, clusterID)
	if err != nil {
		return err
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	return ensureResource(ctx, cs.CoreV1().Namespaces(), name, "namespace "+name, ns, nil)
}

func (r *RealShootAccess) GetNamespace(ctx context.Context, clusterID uuid.UUID, name string) (*ResourceInfo, error) {
	cs, err := r.newClient(ctx, clusterID)
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
	cs, err := r.newClient(ctx, clusterID)
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
	cs, err := r.newClient(ctx, clusterID)
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
	cs, err := r.newClient(ctx, clusterID)
	if err != nil {
		return err
	}

	return deleteResource(ctx, cs.CoreV1().Namespaces(), name, "namespace "+name)
}

func (r *RealShootAccess) ListNamespaces(ctx context.Context, clusterID uuid.UUID, labelKey string) ([]ResourceInfo, error) {
	cs, err := r.newClient(ctx, clusterID)
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
	cs, err := r.newClient(ctx, clusterID)
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
		func(existing *corev1.ServiceAccount) mergeAction {
			mergeMeta(&existing.ObjectMeta, labels, annotations)
			return mergeUpdate
		})
}

func (r *RealShootAccess) EnsureClusterRoleBinding(ctx context.Context, clusterID uuid.UUID, name, roleName, saNamespace, saName string, labels, annotations map[string]string) error {
	cs, err := r.newClient(ctx, clusterID)
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
			Name:     roleName,
		},
	}

	return ensureResource(ctx, cs.RbacV1().ClusterRoleBindings(), name, "CRB "+name, crb,
		func(existing *rbacv1.ClusterRoleBinding) mergeAction {
			if ClusterRoleBindingNeedsRecreate(existing, crb) {
				return mergeRecreate
			}
			mergeMeta(&existing.ObjectMeta, labels, annotations)
			existing.Subjects = crb.Subjects
			return mergeUpdate
		})
}

func (r *RealShootAccess) DeleteServiceAccount(ctx context.Context, clusterID uuid.UUID, namespace, name string) error {
	cs, err := r.newClient(ctx, clusterID)
	if err != nil {
		return err
	}

	return deleteResource(ctx, cs.CoreV1().ServiceAccounts(namespace), name, fmt.Sprintf("SA %s/%s", namespace, name))
}

func (r *RealShootAccess) DeleteClusterRoleBinding(ctx context.Context, clusterID uuid.UUID, name string) error {
	cs, err := r.newClient(ctx, clusterID)
	if err != nil {
		return err
	}

	return deleteResource(ctx, cs.RbacV1().ClusterRoleBindings(), name, "CRB "+name)
}

func (r *RealShootAccess) ListServiceAccounts(ctx context.Context, clusterID uuid.UUID, namespace, labelKey string) ([]ResourceInfo, error) {
	cs, err := r.newClient(ctx, clusterID)
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
	cs, err := r.newClient(ctx, clusterID)
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
	cs, err := r.newClient(ctx, clusterID)
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
		func(existing *corev1.LimitRange) mergeAction {
			mergeMeta(&existing.ObjectMeta, labels, nil)
			existing.Spec = lr.Spec
			return mergeUpdate
		})
}

func (r *RealShootAccess) DeleteLimitRange(ctx context.Context, clusterID uuid.UUID, namespace string) error {
	cs, err := r.newClient(ctx, clusterID)
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

// removedStoredVersions returns the versions the live CRD still lists in
// status.storedVersions that the desired spec no longer serves. Replacing the
// spec while such a version remains is rejected by the API server, because the
// objects persisted under it would become unreadable; retiring a version needs
// a storage migration first, per
// https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning/#upgrade-existing-objects-to-a-new-stored-version
//
// This mirrors SafeStorageVersionUpgrade from Operator Lifecycle Manager
// (operator-framework/operator-lifecycle-manager, pkg/lib/crd/storage.go),
// which blocks an install for the same reason. As there, an empty
// status.storedVersions is treated as safe: there is nothing to strand.
func removedStoredVersions(existing, desired *apiextensionsv1.CustomResourceDefinition) []string {
	desiredVersions := make(map[string]struct{}, len(desired.Spec.Versions))
	for _, v := range desired.Spec.Versions {
		desiredVersions[v.Name] = struct{}{}
	}

	var removed []string
	for _, stored := range existing.Status.StoredVersions {
		if _, ok := desiredVersions[stored]; !ok {
			removed = append(removed, stored)
		}
	}
	return removed
}

func (r *RealShootAccess) EnsureCRD(ctx context.Context, clusterID uuid.UUID, manifest []byte) error {
	cs, err := r.newAPIExtClient(ctx, clusterID)
	if err != nil {
		return err
	}

	crd := &apiextensionsv1.CustomResourceDefinition{}
	err = yaml.Unmarshal(manifest, crd)
	if err != nil {
		return fmt.Errorf("unmarshal CRD manifest: %w", err)
	}

	return ensureResource(ctx, cs.ApiextensionsV1().CustomResourceDefinitions(), crd.Name, "CRD "+crd.Name, crd,
		func(existing *apiextensionsv1.CustomResourceDefinition) mergeAction {
			// Refuse rather than let the API server reject this on every shoot,
			// every tick: EnsureCRD runs fleet-wide, and a hard error here would
			// trip the reconcile worker's consecutive-failure exit.
			if removed := removedStoredVersions(existing, crd); len(removed) > 0 {
				r.logger.Error("refusing CRD update that would drop stored versions",
					"cluster_id", clusterID,
					"crd", crd.Name,
					"removed_stored_versions", removed,
					"remedy", "migrate stored objects and clear status.storedVersions before dropping the version from the chart")
				return mergeSkip
			}
			mergeMeta(&existing.ObjectMeta, crd.Labels, crd.Annotations)
			existing.Spec = crd.Spec
			return mergeUpdate
		})
}

func (r *RealShootAccess) EnsureClusterRole(ctx context.Context, clusterID uuid.UUID, name string, rules []rbacv1.PolicyRule, labels map[string]string) error {
	cs, err := r.newClient(ctx, clusterID)
	if err != nil {
		return err
	}

	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Rules: rules,
	}

	return ensureResource(ctx, cs.RbacV1().ClusterRoles(), name, "ClusterRole "+name, role,
		func(existing *rbacv1.ClusterRole) mergeAction {
			mergeMeta(&existing.ObjectMeta, labels, nil)
			existing.Rules = rules
			return mergeUpdate
		})
}

func (r *RealShootAccess) EnsureDeployment(ctx context.Context, clusterID uuid.UUID, deployment *appsv1.Deployment) error {
	cs, err := r.newClient(ctx, clusterID)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("Deployment %s/%s", deployment.Namespace, deployment.Name)
	return ensureResource(ctx, cs.AppsV1().Deployments(deployment.Namespace), deployment.Name, desc, deployment,
		func(existing *appsv1.Deployment) mergeAction {
			// The label selector is immutable; a changed selector needs delete+recreate.
			if !equality.Semantic.DeepEqual(existing.Spec.Selector, deployment.Spec.Selector) {
				return mergeRecreate
			}
			mergeMeta(&existing.ObjectMeta, deployment.Labels, deployment.Annotations)
			existing.Spec = deployment.Spec
			return mergeUpdate
		})
}

var _ ShootAccess = (*RealShootAccess)(nil)
