package main

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

// DiskInventoryReconciler watches Rook device-discovery ConfigMaps and
// reconciles them into cluster-scoped Disk custom resources.
type DiskInventoryReconciler struct {
	Client        client.Client
	RookNamespace string
	// LoopDevices restricts discovery to loop-backed partitions; see
	// ParseDiscoveredDevices.
	LoopDevices bool
}

// SetupWithManager registers the reconciler with the controller-runtime manager.
//
// The ConfigMap watch is filtered to the Rook namespace's discovery ConfigMaps.
// StoragePools are watched too, mapped back to every discovery ConfigMap: a pool
// being created or deleted changes which disks are claimed, and claimedBy is
// what the console's disk picker filters on. Without this watch a disk stays
// "unclaimed" in the UI until the discovery daemon happens to rewrite its
// ConfigMap (default interval: 60m), long enough for an operator to hand the
// same disk to a second pool.
func (r *DiskInventoryReconciler) SetupWithManager(mgr manager.Manager) error {
	discoverPredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		if obj.GetNamespace() != r.RookNamespace {
			return false
		}
		return obj.GetLabels()["app"] == discoverAppLabel
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}, builder.WithPredicates(discoverPredicate)).
		Watches(
			&v1alpha1.StoragePool{},
			handler.EnqueueRequestsFromMapFunc(r.poolToDiscoveryConfigMaps),
		).
		Complete(r)
}

// discoverAppLabel is the label rook-discover puts on the per-node ConfigMaps
// it writes its probe results to.
const discoverAppLabel = "rook-discover"

// poolToDiscoveryConfigMaps maps a StoragePool event onto every discovery
// ConfigMap, so each node's Disk CRs get their claimedBy recomputed.
func (r *DiskInventoryReconciler) poolToDiscoveryConfigMaps(ctx context.Context, _ client.Object) []reconcile.Request {
	var cms corev1.ConfigMapList
	err := r.Client.List(ctx, &cms,
		client.InNamespace(r.RookNamespace),
		client.MatchingLabels{"app": discoverAppLabel},
	)
	if err != nil {
		// Return empty; the reconciler will retry on the next event.
		return nil
	}
	reqs := make([]reconcile.Request, 0, len(cms.Items))
	for _, cm := range cms.Items {
		reqs = append(reqs, reconcile.Request{
			NamespacedName: types.NamespacedName{Namespace: cm.Namespace, Name: cm.Name},
		})
	}
	return reqs
}

// Reconcile processes a Rook device-discovery ConfigMap and upserts Disk CRs.
func (r *DiskInventoryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cm corev1.ConfigMap
	if err := r.Client.Get(ctx, req.NamespacedName, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get ConfigMap %s: %w", req.NamespacedName, err)
	}

	node := nodeFromConfigMap(cm.Name, cm.Labels)
	raw := cm.Data["devices"]

	statuses, err := ParseDiscoveredDevices(node, raw, r.LoopDevices)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("parse discovered devices for node %q: %w", node, err)
	}

	// Build a lookup of all StoragePools so we can resolve ClaimedBy.
	claimedBy, err := r.buildClaimedByIndex(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("build claimed-by index: %w", err)
	}

	// Upsert a Disk CR for every discovered device.
	seenNames := make(map[string]struct{}, len(statuses))
	for _, st := range statuses {
		name := DiskName(node, st.Path)
		seenNames[name] = struct{}{}
		st.ClaimedBy = claimedBy[name]

		if err := r.upsertDisk(ctx, name, st); err != nil {
			return ctrl.Result{}, fmt.Errorf("upsert disk %q: %w", name, err)
		}
	}

	// Soft-delete Disk CRs belonging to this node that are no longer present.
	if err := r.softDeleteStale(ctx, node, seenNames); err != nil {
		return ctrl.Result{}, fmt.Errorf("soft-delete stale disks for node %q: %w", node, err)
	}

	return ctrl.Result{}, nil
}

// nodeFromConfigMap derives the node name from the ConfigMap's labels or name.
// It is exported as a package-level helper so it can be unit-tested cheaply.
func nodeFromConfigMap(name string, labels map[string]string) string {
	if node, ok := labels["rook.io/node"]; ok && node != "" {
		return node
	}
	return strings.TrimPrefix(name, "local-device-")
}

// buildClaimedByIndex lists all StoragePools and returns a map from disk name
// to the StoragePool entitled to it. Precedence when several pools list the
// same disk is decided by BuildClaimIndex, so the inventory and the StoragePool
// reconciler always agree on who owns what.
func (r *DiskInventoryReconciler) buildClaimedByIndex(ctx context.Context) (map[string]string, error) {
	var pools v1alpha1.StoragePoolList
	if err := r.Client.List(ctx, &pools); err != nil {
		return nil, fmt.Errorf("list StoragePools: %w", err)
	}
	return BuildClaimIndex(pools.Items), nil
}

// upsertDisk creates or updates the Disk object and then writes the status via
// the status subresource.
func (r *DiskInventoryReconciler) upsertDisk(ctx context.Context, name string, st v1alpha1.DiskStatus) error {
	disk := &v1alpha1.Disk{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, disk, func() error {
		// DiskSpec is intentionally empty; nothing to mutate on the spec.
		return nil
	})
	if err != nil {
		return fmt.Errorf("create-or-update Disk %q: %w", name, err)
	}

	// Re-fetch to get the current resource version before patching status.
	var current v1alpha1.Disk
	if err := r.Client.Get(ctx, types.NamespacedName{Name: name}, &current); err != nil {
		return fmt.Errorf("get Disk %q for status update: %w", name, err)
	}
	if current.Status == st {
		return nil // nothing changed; skip the write
	}
	current.Status = st
	if err := r.Client.Status().Update(ctx, &current); err != nil {
		return fmt.Errorf("update status for Disk %q: %w", name, err)
	}
	return nil
}

// softDeleteStale marks any Disk belonging to node that is no longer seen as
// Available=false. It never calls Delete — repo policy is soft deletes only.
func (r *DiskInventoryReconciler) softDeleteStale(ctx context.Context, node string, seen map[string]struct{}) error {
	var allDisks v1alpha1.DiskList
	if err := r.Client.List(ctx, &allDisks); err != nil {
		return fmt.Errorf("list Disks: %w", err)
	}

	for i := range allDisks.Items {
		disk := &allDisks.Items[i]
		if disk.Status.Node != node {
			continue
		}
		if _, ok := seen[disk.Name]; ok {
			continue
		}
		// This disk was on the node but is no longer reported — mark unavailable.
		if disk.Status.Available {
			disk.Status.Available = false
			if err := r.Client.Status().Update(ctx, disk); err != nil {
				return fmt.Errorf("mark disk %q unavailable: %w", disk.Name, err)
			}
		}
	}
	return nil
}
