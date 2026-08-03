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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

// DiskInventoryReconciler watches Rook device-discovery ConfigMaps and
// reconciles them into cluster-scoped Disk custom resources.
type DiskInventoryReconciler struct {
	Client        client.Client
	RookNamespace string
}

// SetupWithManager registers the reconciler with the controller-runtime manager.
// It watches ConfigMaps in all namespaces but filters to only those in the Rook
// namespace with the label app=rook-discover.
func (r *DiskInventoryReconciler) SetupWithManager(mgr manager.Manager) error {
	discoverPredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		if obj.GetNamespace() != r.RookNamespace {
			return false
		}
		return obj.GetLabels()["app"] == "rook-discover"
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		WithEventFilter(discoverPredicate).
		Complete(r)
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

	statuses, err := ParseDiscoveredDevices(node, raw)
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
// to the name of the first StoragePool that references it.
func (r *DiskInventoryReconciler) buildClaimedByIndex(ctx context.Context) (map[string]string, error) {
	var pools v1alpha1.StoragePoolList
	if err := r.Client.List(ctx, &pools); err != nil {
		return nil, fmt.Errorf("list StoragePools: %w", err)
	}
	index := make(map[string]string)
	for _, pool := range pools.Items {
		for _, diskName := range pool.Spec.Disks {
			if _, alreadyClaimed := index[diskName]; !alreadyClaimed {
				index[diskName] = pool.Name
			}
		}
	}
	return index, nil
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
