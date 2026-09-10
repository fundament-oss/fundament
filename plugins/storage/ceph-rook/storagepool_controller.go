package main

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
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
// StoragePool (the primary resource), maps Disk changes to all StoragePools so
// that a disk becoming available or unavailable triggers reconciliation of
// every pool, and maps CephCluster changes the same way so a pool reconciled
// before the singleton existed contributes its disks the moment it appears.
func (r *StoragePoolReconciler) SetupWithManager(mgr manager.Manager) error {
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.StoragePool{}).
		// No predicate: Disk changes arrive via the status subresource, which a
		// generation filter would drop.
		Watches(
			&v1alpha1.Disk{},
			handler.EnqueueRequestsFromMapFunc(r.toAllStoragePools),
		).
		// Only spec changes matter here; without the predicate Rook's ~1/min
		// status heartbeat would fan out to a reconcile of every pool.
		Watches(
			rookStub("CephCluster"),
			handler.EnqueueRequestsFromMapFunc(r.toAllStoragePools),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Complete(r); err != nil {
		return fmt.Errorf("register StoragePool controller: %w", err)
	}
	return nil
}

// toAllStoragePools maps a Disk or CephCluster event to reconcile.Requests for
// every StoragePool: a disk changing availability can shift any pool's
// selection, and a CephCluster appearing means every pool's disks must be
// recorded in it.
func (r *StoragePoolReconciler) toAllStoragePools(ctx context.Context, _ client.Object) []reconcile.Request {
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
	clusterExists, err := r.reconcileCephClusterNodes(ctx)
	if err != nil {
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

	// Ready below means "this pool's disks are recorded in the CephCluster".
	// Without the CephCluster nothing was recorded; its watch re-reconciles
	// every pool the moment it appears.
	if !clusterExists {
		status := pool.Status
		status.Phase = v1alpha1.PhaseDegraded
		// Selection succeeded, so selectedDiskCount reports it. RawCapacityBytes
		// stays 0: it counts contributed disks, and nothing was recorded.
		status.SelectedDiskCount, status.RawCapacityBytes = len(selected), 0
		status.Message = fmt.Sprintf("CephCluster %s/%s not found: disks are contributed once it exists", r.ClusterNamespace, cephClusterName)
		return ctrl.Result{}, r.writeStatus(ctx, pool, &status, &metav1.Condition{
			Status:  metav1.ConditionFalse,
			Reason:  v1alpha1.ReasonCephClusterMissing,
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

// statusWriter builds the shared writer (see status.go) for StoragePool.
func (r *StoragePoolReconciler) statusWriter() statusWriter[*v1alpha1.StoragePool, v1alpha1.StoragePoolStatus, *v1alpha1.StoragePoolStatus] {
	return statusWriter[*v1alpha1.StoragePool, v1alpha1.StoragePoolStatus, *v1alpha1.StoragePoolStatus]{
		client:    r.Client,
		kind:      "StoragePool",
		newObject: func() *v1alpha1.StoragePool { return &v1alpha1.StoragePool{} },
		statusOf:  func(p *v1alpha1.StoragePool) *v1alpha1.StoragePoolStatus { return &p.Status },
	}
}

func (r *StoragePoolReconciler) writeStatus(ctx context.Context, pool *v1alpha1.StoragePool, status *v1alpha1.StoragePoolStatus, ready *metav1.Condition) error {
	return r.statusWriter().write(ctx, pool.Name, status, ready)
}

func (r *StoragePoolReconciler) setDegraded(ctx context.Context, pool *v1alpha1.StoragePool, cause error) {
	r.statusWriter().setDegraded(ctx, pool, cause)
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
// spec.storage.nodes to the union of all live StoragePools' disks. It reports
// whether the CephCluster exists: when it does not, nothing was recorded and
// the caller must not present the pool as contributed.
func (r *StoragePoolReconciler) reconcileCephClusterNodes(ctx context.Context) (clusterExists bool, err error) {
	union, err := diskUnion(ctx, r.Client)
	if err != nil {
		return false, err
	}

	nodes := BuildStorageNodes(union)
	nodesIface := storageNodesToInterface(nodes)

	// Fetch the singleton CephCluster.
	cc := rookStub("CephCluster")
	err = r.Client.Get(ctx, types.NamespacedName{Namespace: r.ClusterNamespace, Name: cephClusterName}, cc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("get CephCluster: %w", err)
	}

	// Skip the identical write, for the same reason writeStatus does.
	if current, found, err := unstructured.NestedSlice(cc.Object, "spec", "storage", "nodes"); err == nil &&
		found && equality.Semantic.DeepEqual(current, nodesIface) {
		return true, nil
	}

	// Set only spec.storage.nodes; do not touch other fields.
	if err := unstructured.SetNestedSlice(cc.Object, nodesIface, "spec", "storage", "nodes"); err != nil {
		return true, fmt.Errorf("set CephCluster spec.storage.nodes: %w", err)
	}

	if err := r.Client.Update(ctx, cc); err != nil {
		return true, fmt.Errorf("update CephCluster: %w", err)
	}
	return true, nil
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
// Terminal: no amount of retrying makes someone else's object ours. The owner
// is left Degraded with this message. Renaming the owner produces a watch
// event via For(); removing the conflicting object produces one via the
// consumer controllers' same-named watches (Owns alone would not see a foreign
// object).
func notOursError(ownerKind, kind, name string) error {
	//nolint:wrapcheck // TerminalError is a wrapper; wrapping it again adds nothing
	return reconcile.TerminalError(fmt.Errorf(
		"%s %q already exists and is not owned by this %s; "+
			"refusing to adopt it (deleting the %s would then delete that object). "+
			"Rename the %s or remove the conflicting %s",
		kind, name, ownerKind, ownerKind, ownerKind, kind))
}
