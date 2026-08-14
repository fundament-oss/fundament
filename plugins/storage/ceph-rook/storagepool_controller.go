package main

import (
	"context"
	"fmt"
	"sort"
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
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

// provisioningRequeue is how often a pool that is not yet Ready re-checks its
// CephBlockPool, rather than waiting out the manager's ~10h resync.
const provisioningRequeue = 30 * time.Second

// StoragePoolReconciler reconciles StoragePool objects into CephBlockPool and
// StorageClass resources, and keeps the singleton CephCluster's
// spec.storage.nodes up to date with the union of all pools' disks.
type StoragePoolReconciler struct {
	Client client.Client
	// ClusterNamespace is where the CephCluster and its CSI secrets live.
	ClusterNamespace string
	// RookNamespace is where the rook operator runs. It names the CSI driver,
	// so it has to reach the StorageClass even though nothing else here uses it.
	RookNamespace string
	Scheme        *runtime.Scheme
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
			// The pool is gone; owner references on CephBlockPool and
			// StorageClass make Kubernetes GC remove those. The CephCluster is
			// not owned by any pool, so its device list has to be recomputed
			// here or it would keep listing the deleted pool's disks until the
			// next resync. controller-runtime delivers the delete event after
			// the cache has dropped the object, so the List below no longer
			// returns it.
			if err := r.reconcileCephClusterNodes(ctx); err != nil {
				return ctrl.Result{}, fmt.Errorf("reconcile CephCluster nodes after deleting StoragePool %s: %w", req.Name, err)
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get StoragePool %s: %w", req.Name, err)
	}

	// A pool being deleted has already released its claims; recompute the union
	// without it and rely on owner-reference GC for the rest.
	if !pool.DeletionTimestamp.IsZero() {
		if err := r.reconcileCephClusterNodes(ctx); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile CephCluster nodes while deleting StoragePool %s: %w", pool.Name, err)
		}
		return ctrl.Result{}, nil
	}

	result, err := r.reconcilePool(ctx, &pool)
	if err != nil {
		// Record why the pool is stuck before surfacing the error, so the
		// operator sees the reason in the console instead of only in the logs.
		r.setDegraded(ctx, &pool, err)
		return ctrl.Result{}, err
	}
	return result, nil
}

// reconcilePool does the work for a live StoragePool.
func (r *StoragePoolReconciler) reconcilePool(ctx context.Context, pool *v1alpha1.StoragePool) (ctrl.Result, error) {
	// Step 2: Resolve this pool's spec.disks into DiskStatus values.
	selected, notes, err := r.resolveDisks(ctx, pool)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("resolve disks for StoragePool %s: %w", pool.Name, err)
	}

	// Step 3: Compute replication from this pool's selected disks.
	nodeCount := DistinctNodeCount(selected)
	replicas, domain, msg := ComputeReplication(pool.Spec.Replication, nodeCount)
	if msg != "" {
		notes = append([]string{msg}, notes...)
	}

	// Step 4: Update the singleton CephCluster's spec.storage.nodes with the
	// union of all pools' disks so that pools don't clobber each other.
	if err := r.reconcileCephClusterNodes(ctx); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconcile CephCluster nodes: %w", err)
	}

	derived := DerivedName(pool.Name)

	// Step 5a: Apply the CephBlockPool owned by this pool.
	cbp := RenderCephBlockPool(r.ClusterNamespace, derived, replicas, domain)
	if err := r.applyBlockPool(ctx, pool, cbp); err != nil {
		return ctrl.Result{}, fmt.Errorf("apply CephBlockPool %s: %w", derived, err)
	}

	// Step 5b: Apply the StorageClass owned by this pool.
	if err := r.applyStorageClass(ctx, pool, derived); err != nil {
		return ctrl.Result{}, fmt.Errorf("apply StorageClass %s: %w", derived, err)
	}

	// Step 6: Read the CephBlockPool's status to determine this pool's phase.
	phase, err := r.blockPoolPhase(ctx, derived)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("read CephBlockPool %s phase: %w", derived, err)
	}

	var rawCapacity int64
	for _, d := range selected {
		rawCapacity += d.SizeBytes
	}

	if err := r.writeStatus(ctx, pool, v1alpha1.StoragePoolStatus{
		Phase:             phase,
		StorageClassName:  derived,
		Replicas:          replicas,
		FailureDomain:     domain,
		SelectedDiskCount: len(selected),
		RawCapacityBytes:  rawCapacity,
		Message:           strings.Join(notes, "; "),
	}); err != nil {
		return ctrl.Result{}, err
	}

	// Requeue while the pool is still provisioning so that we pick up the
	// CephBlockPool becoming Ready without relying on the ~10h default resync.
	if phase != v1alpha1.PhaseReady {
		return ctrl.Result{RequeueAfter: provisioningRequeue}, nil
	}
	return ctrl.Result{}, nil
}

// writeStatus persists status via the status subresource, refetching first so a
// stale resourceVersion from earlier in the reconcile doesn't cause a conflict.
func (r *StoragePoolReconciler) writeStatus(ctx context.Context, pool *v1alpha1.StoragePool, status v1alpha1.StoragePoolStatus) error {
	var current v1alpha1.StoragePool
	if err := r.Client.Get(ctx, types.NamespacedName{Name: pool.Name}, &current); err != nil {
		if apierrors.IsNotFound(err) {
			return nil // deleted mid-reconcile; nothing to record
		}
		return fmt.Errorf("get StoragePool %s for status update: %w", pool.Name, err)
	}
	if current.Status == status {
		// Every Disk event in the cluster fans out to a reconcile of every pool,
		// and the status is identical almost every time. The API server would
		// no-op the write, but not before it has been serialised and sent.
		return nil
	}
	current.Status = status
	if err := r.Client.Status().Update(ctx, &current); err != nil {
		return fmt.Errorf("update StoragePool status %s: %w", pool.Name, err)
	}
	return nil
}

// setDegraded records a reconcile failure on the pool. Status write failures are
// logged rather than returned: the caller is already returning the real error,
// and masking it with a status-write error would hide the cause.
func (r *StoragePoolReconciler) setDegraded(ctx context.Context, pool *v1alpha1.StoragePool, cause error) {
	status := pool.Status
	status.Phase = v1alpha1.PhaseDegraded
	status.Message = cause.Error()
	if err := r.writeStatus(ctx, pool, status); err != nil {
		log.FromContext(ctx).Error(err, "could not record degraded status", "storagePool", pool.Name)
	}
}

// resolveDisks fetches DiskStatus for each disk this pool selects, and returns
// human-readable notes for anything it had to skip.
//
// Disks that do not exist are skipped. Disks another StoragePool has a stronger
// claim to are skipped too — silently feeding them into this pool would put the
// same device in two pools' capacity accounting. Any other error (e.g. a
// transient API failure) is returned so the caller can requeue, rather than
// silently dropping the disk and lowering the disk count.
//
// A disk reporting available=false is NOT skipped: once Ceph consumes a device
// it stops looking empty, so dropping unavailable disks would pull live OSDs
// out of the CephCluster on the next reconcile.
func (r *StoragePoolReconciler) resolveDisks(ctx context.Context, pool *v1alpha1.StoragePool) ([]v1alpha1.DiskStatus, []string, error) {
	var pools v1alpha1.StoragePoolList
	if err := r.Client.List(ctx, &pools); err != nil {
		return nil, nil, fmt.Errorf("list StoragePools: %w", err)
	}

	var (
		selected  []v1alpha1.DiskStatus
		missing   []string
		conflicts []string
	)
	// spec.disks is a set (x-kubernetes-list-type on the CRD), but an object
	// written before that marker existed could still repeat a name, and a repeat
	// would be counted twice in selectedDiskCount and rawCapacityBytes -- the
	// two numbers an operator sizes workloads against.
	seen := make(map[string]struct{}, len(pool.Spec.Disks))
	for _, name := range pool.Spec.Disks {
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		if owner := ClaimOwner(pools.Items, name); owner != "" && owner != pool.Name {
			conflicts = append(conflicts, fmt.Sprintf("%s (claimed by %s)", name, owner))
			continue
		}
		var disk v1alpha1.Disk
		err := r.Client.Get(ctx, types.NamespacedName{Name: name}, &disk)
		if apierrors.IsNotFound(err) {
			missing = append(missing, name)
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("get Disk %q: %w", name, err)
		}
		selected = append(selected, disk.Status)
	}

	var notes []string
	if len(missing) > 0 {
		notes = append(notes, "skipped missing disks: "+strings.Join(missing, ", "))
	}
	if len(conflicts) > 0 {
		notes = append(notes, "skipped disks claimed by another pool: "+strings.Join(conflicts, ", "))
	}
	return selected, notes, nil
}

// reconcileCephClusterNodes loads the singleton CephCluster and sets
// spec.storage.nodes to the union of all live StoragePools' disks.
func (r *StoragePoolReconciler) reconcileCephClusterNodes(ctx context.Context) error {
	var pools v1alpha1.StoragePoolList
	if err := r.Client.List(ctx, &pools); err != nil {
		return fmt.Errorf("list StoragePools: %w", err)
	}

	// Deduplicate by (node, path) so a disk listed in multiple pools isn't doubled.
	seen := make(map[string]struct{})
	var union []v1alpha1.DiskStatus
	for _, pool := range pools.Items {
		// A pool being deleted has released its disks; leaving them in would
		// keep Rook configured for devices no pool owns any more.
		if !pool.DeletionTimestamp.IsZero() {
			continue
		}
		for _, name := range pool.Spec.Disks {
			var disk v1alpha1.Disk
			err := r.Client.Get(ctx, types.NamespacedName{Name: name}, &disk)
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return fmt.Errorf("get Disk %q: %w", name, err)
			}
			// Keyed on the same reference that goes into the CephCluster, so two
			// Disk CRs that resolve to one device collapse to one entry.
			key := disk.Status.Node + "\x00" + DeviceRef(disk.Status)
			if _, ok := seen[key]; !ok {
				seen[key] = struct{}{}
				union = append(union, disk.Status)
			}
		}
	}

	nodes := BuildStorageNodes(union)
	nodesIface := storageNodesToInterface(nodes)

	// Fetch the singleton CephCluster.
	cc := &unstructured.Unstructured{}
	cc.SetAPIVersion(cephAPIVersion)
	cc.SetKind("CephCluster")
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: r.ClusterNamespace, Name: cephClusterName}, cc)
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
		apiVersion = v1alpha1.GroupVersion.String()
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

// notOursError reports an existing object this pool is not allowed to touch.
//
// Terminal: no amount of retrying makes someone else's object ours. The pool is
// left Degraded with this message, and the operator renaming the pool or
// removing the conflicting object produces a watch event that reconciles it
// again -- which is the only thing that can resolve it.
func notOursError(kind, name string) error {
	return reconcile.TerminalError(fmt.Errorf(
		"%s %q already exists and is not owned by this StoragePool; "+
			"refusing to adopt it (deleting the pool would then delete that object). "+
			"Rename the StoragePool or remove the conflicting %s",
		kind, name, kind))
}

// applyBlockPool creates or updates the CephBlockPool and sets the StoragePool
// as its controller owner. An existing CephBlockPool that this pool does not
// already own is left alone: Rook keeps its own pools (".mgr" and friends) in
// this namespace, and adopting one would delete it when the pool goes away.
func (r *StoragePoolReconciler) applyBlockPool(ctx context.Context, pool *v1alpha1.StoragePool, cbp *unstructured.Unstructured) error {
	cbp.SetOwnerReferences([]metav1.OwnerReference{poolOwnerRef(pool)})

	existing := &unstructured.Unstructured{}
	existing.SetAPIVersion(cephAPIVersion)
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

	if !ownedByPool(existing.GetOwnerReferences(), pool) {
		return notOursError("CephBlockPool", cbp.GetName())
	}

	// Update spec and owner refs while preserving the existing resource version.
	existing.Object["spec"] = cbp.Object["spec"]
	existing.SetOwnerReferences(cbp.GetOwnerReferences())
	if err := r.Client.Update(ctx, existing); err != nil {
		return fmt.Errorf("update CephBlockPool: %w", err)
	}
	return nil
}

// applyStorageClass creates the StorageClass, or reconciles the mutable parts of
// one this pool already owns.
//
// StorageClass is almost entirely immutable: the API server rejects updates to
// parameters, provisioner, reclaimPolicy and volumeBindingMode alike (only
// allowVolumeExpansion and metadata can change). So rather than issuing an
// update the API server will refuse forever, drift on those fields is reported
// as Degraded with the one action that fixes it.
func (r *StoragePoolReconciler) applyStorageClass(ctx context.Context, pool *v1alpha1.StoragePool, name string) error {
	desired := RenderStorageClass(name, r.ClusterNamespace, name, r.RookNamespace)

	var existing storagev1.StorageClass
	err := r.Client.Get(ctx, types.NamespacedName{Name: name}, &existing)
	if apierrors.IsNotFound(err) {
		if err := controllerutil.SetControllerReference(pool, desired, r.Scheme); err != nil {
			return fmt.Errorf("set owner reference: %w", err)
		}
		if err := r.Client.Create(ctx, desired); err != nil {
			return fmt.Errorf("create StorageClass: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get StorageClass: %w", err)
	}

	if !ownedByPool(existing.OwnerReferences, pool) {
		return notOursError("StorageClass", name)
	}

	// Terminal for the same reason as notOursError: the API server will refuse
	// this update every time, so retrying only burns backoff and fills the log.
	if drift := immutableStorageClassDrift(&existing, desired); len(drift) > 0 {
		return reconcile.TerminalError(fmt.Errorf(
			"StorageClass %q differs from the desired spec on immutable field(s) %s; "+
				"Kubernetes does not allow updating these. Delete the StorageClass to have it recreated "+
				"(existing PersistentVolumes keep working; only new provisioning uses it)",
			name, strings.Join(drift, ", ")))
	}

	// Only the mutable fields, plus the owner reference so cascade deletion
	// stays wired even if it was stripped.
	existing.AllowVolumeExpansion = desired.AllowVolumeExpansion
	if err := controllerutil.SetControllerReference(pool, &existing, r.Scheme); err != nil {
		return fmt.Errorf("set owner reference: %w", err)
	}
	if err := r.Client.Update(ctx, &existing); err != nil {
		return fmt.Errorf("update StorageClass: %w", err)
	}
	return nil
}

// immutableStorageClassDrift names the immutable fields on which existing and
// desired disagree. Kubernetes' ValidateStorageClassUpdate forbids changing any
// of them.
func immutableStorageClassDrift(existing, desired *storagev1.StorageClass) []string {
	var drift []string
	if existing.Provisioner != desired.Provisioner {
		drift = append(drift, "provisioner")
	}
	if !mapsEqual(existing.Parameters, desired.Parameters) {
		drift = append(drift, "parameters")
	}
	if !ptrEqual(existing.ReclaimPolicy, desired.ReclaimPolicy) {
		drift = append(drift, "reclaimPolicy")
	}
	if !ptrEqual(existing.VolumeBindingMode, desired.VolumeBindingMode) {
		drift = append(drift, "volumeBindingMode")
	}
	sort.Strings(drift)
	return drift
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

func ptrEqual[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// blockPoolPhase reads the CephBlockPool's status.phase to derive this pool's
// Phase. A CephBlockPool that does not exist yet is Provisioning; any other
// error is returned rather than reported as Provisioning, so an RBAC or API
// failure surfaces instead of looping silently.
func (r *StoragePoolReconciler) blockPoolPhase(ctx context.Context, name string) (string, error) {
	existing := &unstructured.Unstructured{}
	existing.SetAPIVersion(cephAPIVersion)
	existing.SetKind("CephBlockPool")
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: r.ClusterNamespace, Name: name}, existing)
	if apierrors.IsNotFound(err) {
		return v1alpha1.PhaseProvisioning, nil
	}
	if err != nil {
		return "", fmt.Errorf("get CephBlockPool: %w", err)
	}
	phase, _, _ := unstructured.NestedString(existing.Object, "status", "phase")
	if phase == v1alpha1.PhaseReady {
		return v1alpha1.PhaseReady, nil
	}
	return v1alpha1.PhaseProvisioning, nil
}
