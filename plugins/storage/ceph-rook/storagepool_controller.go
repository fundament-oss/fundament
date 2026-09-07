package main

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

// StoragePoolReconciler reconciles StoragePool disk contributions into the
// singleton CephCluster's spec.storage.nodes. It owns no derived objects;
// consumer kinds (e.g. BlockStorage) derive StorageClasses from the shared OSD
// set this produces.
type StoragePoolReconciler struct {
	Client client.Client
	// ClusterNamespace is where the CephCluster lives.
	ClusterNamespace string
}

// SetupWithManager registers the controller with the manager. It watches
// StoragePool (the primary resource) and maps Disk changes to all
// StoragePools so that a disk becoming available or unavailable triggers
// reconciliation of every pool.
func (r *StoragePoolReconciler) SetupWithManager(mgr manager.Manager) error {
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.StoragePool{}).
		Watches(
			&v1alpha1.Disk{},
			handler.EnqueueRequestsFromMapFunc(r.diskToStoragePools),
		).
		Complete(r); err != nil {
		return fmt.Errorf("register StoragePool controller: %w", err)
	}
	return nil
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
	for i := range pools.Items {
		reqs = append(reqs, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: pools.Items[i].Name},
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
			// The pool is gone. The CephCluster is not owned by any pool, so its
			// device list has to be recomputed here or it would keep listing the
			// deleted pool's disks until the next resync. controller-runtime
			// delivers the delete event after the cache has dropped the object,
			// so the List below no longer returns it.
			if _, err := r.reconcileCephClusterNodes(ctx); err != nil {
				return ctrl.Result{}, fmt.Errorf("reconcile CephCluster nodes after deleting StoragePool %s: %w", req.Name, err)
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get StoragePool %s: %w", req.Name, err)
	}

	// A pool being deleted has already released its claims; recompute the union
	// without it and rely on owner-reference GC for the rest.
	if !pool.DeletionTimestamp.IsZero() {
		if _, err := r.reconcileCephClusterNodes(ctx); err != nil {
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
	// Step 2: Resolve this pool's spec.disks. This is its contribution to the
	// shared OSD set, and what selectedDiskCount/rawCapacityBytes report.
	selected, notes, err := r.resolveDisks(ctx, pool)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("resolve disks for StoragePool %s: %w", pool.Name, err)
	}

	// Step 3: Set the singleton CephCluster's spec.storage.nodes to the union of
	// all pools' disks, so pools don't clobber each other.
	if _, err := r.reconcileCephClusterNodes(ctx); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconcile CephCluster nodes: %w", err)
	}

	// A pool over zero disks contributes nothing to the cluster. Report it
	// instead of writing a Ready status that describes no capacity; the Disk and
	// StoragePool watches will wake us when it can be fixed.
	if len(selected) == 0 {
		status := pool.Status
		status.Phase = v1alpha1.PhaseDegraded
		status.SelectedDiskCount, status.RawCapacityBytes = 0, 0
		status.Message = emptyPoolMessage(pool, notes)
		return ctrl.Result{}, r.writeStatus(ctx, pool, &status, &metav1.Condition{
			Status:  metav1.ConditionFalse,
			Reason:  v1alpha1.ReasonNoUsableDisks,
			Message: status.Message,
		})
	}

	var rawCapacity int64
	for i := range selected {
		rawCapacity += selected[i].SizeBytes
	}

	message := strings.Join(notes, "; ")
	ready := metav1.Condition{
		Status:  metav1.ConditionTrue,
		Reason:  v1alpha1.ReasonReady,
		Message: message,
	}
	if ready.Message == "" {
		ready.Message = "disks contributed to the shared Ceph cluster"
	}
	return ctrl.Result{}, r.writeStatus(ctx, pool, &v1alpha1.StoragePoolStatus{
		Phase:             v1alpha1.PhaseReady,
		SelectedDiskCount: len(selected),
		RawCapacityBytes:  rawCapacity,
		Message:           message,
	}, &ready)
}

// emptyPoolMessage explains why a pool resolved no disks. An empty spec.disks
// produces no notes, which would otherwise leave the message blank.
func emptyPoolMessage(pool *v1alpha1.StoragePool, notes []string) string {
	if len(notes) > 0 {
		return "no usable disks: " + strings.Join(notes, "; ")
	}
	if len(pool.Spec.Disks) == 0 {
		return "no disks selected: add disks to spec.disks"
	}
	return "no usable disks"
}

// writeStatus persists status via the status subresource, refetching first so a
// stale resourceVersion from earlier in the reconcile doesn't cause a conflict.
//
// ready is upserted as the pool's Ready condition. Conditions and
// observedGeneration are taken from the refetched object rather than from the
// caller's status value: the caller builds the observable fields, this decides
// what generation they describe.
func (r *StoragePoolReconciler) writeStatus(ctx context.Context, pool *v1alpha1.StoragePool, status *v1alpha1.StoragePoolStatus, ready *metav1.Condition) error {
	var current v1alpha1.StoragePool
	if err := r.Client.Get(ctx, types.NamespacedName{Name: pool.Name}, &current); err != nil {
		if apierrors.IsNotFound(err) {
			return nil // deleted mid-reconcile; nothing to record
		}
		return fmt.Errorf("get StoragePool %s for status update: %w", pool.Name, err)
	}

	status.ObservedGeneration = current.Generation
	// Carry the existing conditions so SetStatusCondition can preserve
	// lastTransitionTime for a condition whose status has not flipped. Cloned,
	// not aliased: SetStatusCondition mutates the existing element through a
	// pointer into the backing array, so sharing it with current would mutate
	// both sides and the DeepEqual below could never see a condition change.
	status.Conditions = slices.Clone(current.Status.Conditions)
	ready.Type = v1alpha1.ConditionReady
	ready.ObservedGeneration = current.Generation
	meta.SetStatusCondition(&status.Conditions, *ready)

	// Every Disk event in the cluster fans out to a reconcile of every pool, and
	// the status is identical almost every time. The API server would no-op the
	// write, but not before it has been serialised and sent. Compared semantically
	// rather than with ==, which stopped compiling once Conditions was added and
	// would silently compare slice headers if it ever did.
	if equality.Semantic.DeepEqual(current.Status, *status) {
		return nil
	}
	current.Status = *status
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
	ready := metav1.Condition{
		Status:  metav1.ConditionFalse,
		Reason:  v1alpha1.ReasonReconcileError,
		Message: cause.Error(),
	}
	if err := r.writeStatus(ctx, pool, &status, &ready); err != nil {
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
// spec.storage.nodes to the union of all live StoragePools' disks. It returns
// that union: the disks behind every OSD in the cluster, which is what callers
// size replication against.
func (r *StoragePoolReconciler) reconcileCephClusterNodes(ctx context.Context) ([]v1alpha1.DiskStatus, error) {
	union, err := diskUnion(ctx, r.Client)
	if err != nil {
		return nil, err
	}

	nodes := BuildStorageNodes(union)
	nodesIface := storageNodesToInterface(nodes)

	// Fetch the singleton CephCluster.
	cc := &unstructured.Unstructured{}
	cc.SetAPIVersion(cephAPIVersion)
	cc.SetKind("CephCluster")
	err = r.Client.Get(ctx, types.NamespacedName{Namespace: r.ClusterNamespace, Name: cephClusterName}, cc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Not created yet. The union still stands; it describes the pools.
			return union, nil
		}
		return nil, fmt.Errorf("get CephCluster: %w", err)
	}

	// Skip the identical write, for the same reason writeStatus does.
	if current, found, err := unstructured.NestedSlice(cc.Object, "spec", "storage", "nodes"); err == nil &&
		found && equality.Semantic.DeepEqual(current, nodesIface) {
		return union, nil
	}

	// Set only spec.storage.nodes; do not touch other fields.
	if err := unstructured.SetNestedSlice(cc.Object, nodesIface, "spec", "storage", "nodes"); err != nil {
		return nil, fmt.Errorf("set CephCluster spec.storage.nodes: %w", err)
	}

	if err := r.Client.Update(ctx, cc); err != nil {
		return nil, fmt.Errorf("update CephCluster: %w", err)
	}
	return union, nil
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

// controllerRef builds a controller OwnerReference to a storage.fundament.io
// object. Built manually rather than via controllerutil so it can be attached
// to unstructured Rook objects whose GVK is not in the scheme.
func controllerRef(owner metav1.Object, kind string) metav1.OwnerReference {
	isController := true
	blockOwner := true
	return metav1.OwnerReference{
		APIVersion:         v1alpha1.GroupVersion.String(),
		Kind:               kind,
		Name:               owner.GetName(),
		UID:                owner.GetUID(),
		Controller:         &isController,
		BlockOwnerDeletion: &blockOwner,
	}
}

// notOursError reports an existing object this owner is not allowed to touch.
//
// Terminal: no amount of retrying makes someone else's object ours. The owner is
// left Degraded with this message, and the operator renaming the owner or
// removing the conflicting object produces a watch event that reconciles it
// again -- which is the only thing that can resolve it.
func notOursError(ownerKind, kind, name string) error {
	//nolint:wrapcheck // TerminalError is a wrapper; wrapping it again adds nothing
	return reconcile.TerminalError(fmt.Errorf(
		"%s %q already exists and is not owned by this %s; "+
			"refusing to adopt it (deleting the %s would then delete that object). "+
			"Rename the %s or remove the conflicting %s",
		kind, name, ownerKind, ownerKind, ownerKind, kind))
}
