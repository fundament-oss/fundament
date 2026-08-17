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

// SetupWithManager registers the reconciler with the manager.
//
// StoragePools are watched too, mapped back to every discovery ConfigMap, because
// a pool changes which disks are claimed and claimedBy is what the console's
// picker filters on. Without it a disk stays "unclaimed" in the UI until the
// discovery daemon rewrites its ConfigMap (default 60m) -- long enough to hand
// the same disk to a second pool.
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

// discoverAppLabel is what rook-discover labels its per-node ConfigMaps with.
const discoverAppLabel = "rook-discover"

// poolToDiscoveryConfigMaps maps a StoragePool event onto every discovery
// ConfigMap, so every node's claimedBy is recomputed.
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
			// The node left, so rook deleted its ConfigMap. Without this the
			// disks stay available=true forever and the console keeps offering
			// them. The labels went with the object; the name still has the node.
			node := nodeFromConfigMap(req.Name, nil)
			if err := r.softDeleteStale(ctx, node, nil); err != nil {
				return ctrl.Result{}, fmt.Errorf("soft-delete disks for departed node %q: %w", node, err)
			}
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

	claimedBy, err := r.buildClaimedByIndex(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("build claimed-by index: %w", err)
	}

	seenNames := make(map[string]struct{}, len(statuses))
	for _, st := range statuses {
		name := DiskName(node, DeviceKey(st))
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
func nodeFromConfigMap(name string, labels map[string]string) string {
	if node, ok := labels["rook.io/node"]; ok && node != "" {
		return node
	}
	return strings.TrimPrefix(name, "local-device-")
}

// buildClaimedByIndex maps disk name to the StoragePool entitled to it. Uses
// BuildClaimIndex, so the inventory and the StoragePool reconciler always agree.
func (r *DiskInventoryReconciler) buildClaimedByIndex(ctx context.Context) (map[string]string, error) {
	var pools v1alpha1.StoragePoolList
	if err := r.Client.List(ctx, &pools); err != nil {
		return nil, fmt.Errorf("list StoragePools: %w", err)
	}
	return BuildClaimIndex(pools.Items), nil
}

// upsertDisk creates or updates the Disk, then writes status.
func (r *DiskInventoryReconciler) upsertDisk(ctx context.Context, name string, st v1alpha1.DiskStatus) error {
	disk := &v1alpha1.Disk{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, disk, func() error {
		return nil
	})
	if err != nil {
		return fmt.Errorf("create-or-update Disk %q: %w", name, err)
	}

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

// softDeleteStale marks Disks on node that are no longer reported as
// Available=false. Never Delete -- repo policy is soft deletes only. A nil seen
// set means nothing is reported, which is what a departed node looks like.
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
		if disk.Status.Available {
			disk.Status.Available = false
			if err := r.Client.Status().Update(ctx, disk); err != nil {
				return fmt.Errorf("mark disk %q unavailable: %w", disk.Name, err)
			}
		}
	}
	return nil
}
