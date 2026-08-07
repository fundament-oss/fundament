package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

// StoragePoolReconciler reconciles StoragePool objects into CephBlockPool and
// StorageClass resources, and keeps the singleton CephCluster's
// spec.storage.nodes up to date with the union of all pools' disks.
type StoragePoolReconciler struct {
	Client           client.Client
	ClusterNamespace string
	Scheme           *runtime.Scheme
}

// SetupWithManager registers the controller with the manager. It watches
// StoragePool (the primary resource), owns StorageClass objects so that
// deletions cascade, and maps Disk changes to all StoragePools so that a disk
// becoming available or unavailable triggers reconciliation of every pool.
func (r *StoragePoolReconciler) SetupWithManager(mgr manager.Manager) error {
	if r.Scheme == nil {
		r.Scheme = mgr.GetScheme()
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.StoragePool{}).
		Owns(&storagev1.StorageClass{}).
		Watches(
			&v1alpha1.Disk{},
			handler.EnqueueRequestsFromMapFunc(r.diskToStoragePools),
		).
		Complete(r)
}

// diskToStoragePools maps a Disk event to reconcile.Requests for every
// StoragePool. When a disk changes its availability, all pools may be affected
// (node count and therefore replication may change).
func (r *StoragePoolReconciler) diskToStoragePools(ctx context.Context, _ client.Object) []reconcile.Request {
	var pools v1alpha1.StoragePoolList
	if err := r.Client.List(ctx, &pools); err != nil {
		// Return empty; the reconciler will retry on the next event.
		return nil
	}
	reqs := make([]reconcile.Request, 0, len(pools.Items))
	for _, pool := range pools.Items {
		reqs = append(reqs, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: pool.Name},
		})
	}
	return reqs
}

// Reconcile implements reconcile.Reconciler.
func (r *StoragePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Step 1: Fetch the StoragePool (cluster-scoped, no namespace).
	var pool v1alpha1.StoragePool
	if err := r.Client.Get(ctx, req.NamespacedName, &pool); err != nil {
		if apierrors.IsNotFound(err) {
			// Object is gone; owner references on CephBlockPool and StorageClass
			// ensure Kubernetes GC removes them automatically.
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get StoragePool %s: %w", req.Name, err)
	}

	// If the pool is being deleted, rely on owner-reference GC.
	if !pool.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	// Step 2: Resolve this pool's spec.disks into DiskStatus values.
	selected, skippedNote, err := r.resolveDisks(ctx, pool.Spec.Disks)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("resolve disks for StoragePool %s: %w", pool.Name, err)
	}

	// Step 3: Compute replication from this pool's selected disks.
	nodeCount := DistinctNodeCount(selected)
	replicas, domain, msg := ComputeReplication(pool.Spec.Replication, nodeCount)
	if msg != "" && skippedNote != "" {
		msg = msg + "; " + skippedNote
	} else if skippedNote != "" {
		msg = skippedNote
	}

	// Step 4: Update the singleton CephCluster's spec.storage.nodes with the
	// union of all pools' disks so that pools don't clobber each other.
	if err := r.reconcileCephClusterNodes(ctx); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconcile CephCluster nodes: %w", err)
	}

	// Step 5a: Apply the CephBlockPool owned by this pool.
	cbp := RenderCephBlockPool(r.ClusterNamespace, pool.Name, replicas, domain)
	if err := r.applyBlockPool(ctx, &pool, cbp); err != nil {
		return ctrl.Result{}, fmt.Errorf("apply CephBlockPool %s: %w", pool.Name, err)
	}

	// Step 5b: Apply the StorageClass owned by this pool.
	sc := RenderStorageClass(pool.Name, r.ClusterNamespace, pool.Name)
	if err := r.applyStorageClass(ctx, &pool, sc); err != nil {
		return ctrl.Result{}, fmt.Errorf("apply StorageClass %s: %w", pool.Name, err)
	}

	// Step 6: Read the CephBlockPool's status to determine this pool's phase.
	phase := r.blockPoolPhase(ctx, pool.Name)

	// Compute capacity from selected disks.
	var capacityBytes int64
	for _, d := range selected {
		capacityBytes += d.SizeBytes
	}

	// Write status via the status subresource.
	pool.Status = v1alpha1.StoragePoolStatus{
		Phase:            phase,
		StorageClassName: pool.Name,
		Replicas:         replicas,
		FailureDomain:    domain,
		OSDCount:         len(selected),
		CapacityBytes:    capacityBytes,
		Message:          msg,
	}
	if err := r.Client.Status().Update(ctx, &pool); err != nil {
		return ctrl.Result{}, fmt.Errorf("update StoragePool status %s: %w", pool.Name, err)
	}

	// Requeue while the pool is still provisioning so that we pick up the
	// CephBlockPool becoming Ready without relying on the ~10h default resync.
	if phase != "Ready" {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}

// resolveDisks fetches DiskStatus for each named disk. Disks that do not exist
// (NotFound) are skipped and reported in the returned note. Any other error
// (e.g. a transient etcd/API failure) is returned so the caller can requeue,
// rather than silently dropping the disk and lowering the OSD count.
func (r *StoragePoolReconciler) resolveDisks(ctx context.Context, names []string) ([]v1alpha1.DiskStatus, string, error) {
	var (
		selected []v1alpha1.DiskStatus
		missing  []string
	)
	for _, name := range names {
		var disk v1alpha1.Disk
		err := r.Client.Get(ctx, types.NamespacedName{Name: name}, &disk)
		if apierrors.IsNotFound(err) {
			missing = append(missing, name)
			continue
		}
		if err != nil {
			return nil, "", fmt.Errorf("get Disk %q: %w", name, err)
		}
		selected = append(selected, disk.Status)
	}
	var note string
	if len(missing) > 0 {
		note = fmt.Sprintf("skipped missing disks: %s", strings.Join(missing, ", "))
	}
	return selected, note, nil
}

// reconcileCephClusterNodes loads the singleton CephCluster and sets
// spec.storage.nodes to the union of all StoragePools' disks.
func (r *StoragePoolReconciler) reconcileCephClusterNodes(ctx context.Context) error {
	// List all StoragePools and gather the union of their disk statuses.
	var pools v1alpha1.StoragePoolList
	if err := r.Client.List(ctx, &pools); err != nil {
		return fmt.Errorf("list StoragePools: %w", err)
	}

	// Deduplicate by (node, path) so a disk listed in multiple pools isn't doubled.
	seen := make(map[string]struct{})
	var union []v1alpha1.DiskStatus
	for _, pool := range pools.Items {
		disks, _, err := r.resolveDisks(ctx, pool.Spec.Disks)
		if err != nil {
			return fmt.Errorf("resolve disks for StoragePool %s: %w", pool.Name, err)
		}
		for _, d := range disks {
			key := d.Node + "\x00" + d.Path
			if _, ok := seen[key]; !ok {
				seen[key] = struct{}{}
				union = append(union, d)
			}
		}
	}

	nodes := BuildStorageNodes(union)
	nodesIface := storageNodesToInterface(nodes)

	// Fetch the singleton CephCluster.
	cc := &unstructured.Unstructured{}
	cc.SetAPIVersion("ceph.rook.io/v1")
	cc.SetKind("CephCluster")
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: r.ClusterNamespace, Name: "rook-ceph"}, cc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// CephCluster not yet created; skip for now.
			return nil
		}
		return fmt.Errorf("get CephCluster: %w", err)
	}

	// Set only spec.storage.nodes; do not touch other fields.
	if err := unstructured.SetNestedSlice(cc.Object, nodesIface, "spec", "storage", "nodes"); err != nil {
		return fmt.Errorf("set CephCluster spec.storage.nodes: %w", err)
	}

	if err := r.Client.Update(ctx, cc); err != nil {
		return fmt.Errorf("update CephCluster: %w", err)
	}
	return nil
}

// storageNodesToInterface converts []map[string]any to []interface{} as
// required by unstructured.SetNestedSlice. Each inner map element is also
// converted because the slice element type must be interface{}, not map[string]any.
func storageNodesToInterface(nodes []map[string]any) []interface{} {
	if nodes == nil {
		return []interface{}{}
	}
	result := make([]interface{}, len(nodes))
	for i, node := range nodes {
		result[i] = mapAnyToInterface(node)
	}
	return result
}

// mapAnyToInterface recursively converts map[string]any (and any nested
// []map[string]any) to map[string]interface{} / []interface{} so that
// unstructured.SetNestedSlice accepts the value.
func mapAnyToInterface(m map[string]any) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case []map[string]any:
			iface := make([]interface{}, len(val))
			for i, item := range val {
				iface[i] = mapAnyToInterface(item)
			}
			out[k] = iface
		default:
			out[k] = v
		}
	}
	return out
}

// poolOwnerRef builds a metav1.OwnerReference pointing to a StoragePool. We
// build it manually rather than via controllerutil.SetControllerReference so
// that we can attach it to an unstructured CephBlockPool whose GVK is not
// registered in the scheme.
func poolOwnerRef(pool *v1alpha1.StoragePool) metav1.OwnerReference {
	isController := true
	blockOwner := true
	apiVersion := pool.APIVersion
	if apiVersion == "" {
		apiVersion = "storage.fundament.io/v1alpha1"
	}
	kind := pool.Kind
	if kind == "" {
		kind = "StoragePool"
	}
	return metav1.OwnerReference{
		APIVersion:         apiVersion,
		Kind:               kind,
		Name:               pool.Name,
		UID:                pool.UID,
		Controller:         &isController,
		BlockOwnerDeletion: &blockOwner,
	}
}

// applyBlockPool creates or updates the CephBlockPool and sets the StoragePool
// as its controller owner.
func (r *StoragePoolReconciler) applyBlockPool(ctx context.Context, pool *v1alpha1.StoragePool, cbp *unstructured.Unstructured) error {
	cbp.SetOwnerReferences([]metav1.OwnerReference{poolOwnerRef(pool)})

	existing := &unstructured.Unstructured{}
	existing.SetAPIVersion("ceph.rook.io/v1")
	existing.SetKind("CephBlockPool")
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: cbp.GetNamespace(), Name: cbp.GetName()}, existing)
	if apierrors.IsNotFound(err) {
		if err := r.Client.Create(ctx, cbp); err != nil {
			return fmt.Errorf("create CephBlockPool: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get CephBlockPool: %w", err)
	}

	// Update spec and owner refs while preserving the existing resource version.
	existing.Object["spec"] = cbp.Object["spec"]
	existing.SetOwnerReferences(cbp.GetOwnerReferences())
	if err := r.Client.Update(ctx, existing); err != nil {
		return fmt.Errorf("update CephBlockPool: %w", err)
	}
	return nil
}

// applyStorageClass creates or updates the StorageClass and sets the
// StoragePool as its controller owner. The owner reference is applied inside the
// CreateOrUpdate mutate closure: CreateOrUpdate does an internal Get that
// overwrites sc with the live object before running the closure, so setting the
// owner ref beforehand would be lost on the update path and break cascade GC.
func (r *StoragePoolReconciler) applyStorageClass(ctx context.Context, pool *v1alpha1.StoragePool, sc *storagev1.StorageClass) error {
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, sc, func() error {
		// Overwrite the desired fields from a freshly rendered spec.
		rendered := RenderStorageClass(pool.Name, r.ClusterNamespace, pool.Name)
		// Provisioner is immutable on an existing StorageClass; only set it on
		// create (CreationTimestamp is zero for an object not yet persisted).
		if sc.CreationTimestamp.IsZero() {
			sc.Provisioner = rendered.Provisioner
		}
		sc.Parameters = rendered.Parameters
		sc.ReclaimPolicy = rendered.ReclaimPolicy
		sc.AllowVolumeExpansion = rendered.AllowVolumeExpansion
		sc.VolumeBindingMode = rendered.VolumeBindingMode
		// Re-apply the owner reference on every reconcile (create and update),
		// so cascade deletion always stays wired.
		return controllerutil.SetControllerReference(pool, sc, r.Scheme)
	})
	if err != nil {
		return fmt.Errorf("create-or-update StorageClass: %w", err)
	}
	return nil
}

// blockPoolPhase reads the CephBlockPool's status.phase to derive this pool's
// Phase. Returns "Ready" only when the CephBlockPool reports "Ready".
func (r *StoragePoolReconciler) blockPoolPhase(ctx context.Context, name string) string {
	existing := &unstructured.Unstructured{}
	existing.SetAPIVersion("ceph.rook.io/v1")
	existing.SetKind("CephBlockPool")
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: r.ClusterNamespace, Name: name}, existing)
	if err != nil {
		return "Provisioning"
	}
	phase, _, _ := unstructured.NestedString(existing.Object, "status", "phase")
	if phase == "Ready" {
		return "Ready"
	}
	return "Provisioning"
}
