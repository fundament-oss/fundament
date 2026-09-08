package main

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"strings"

	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// applyOwnedStorageClass creates desired, or reconciles the mutable parts of a
// StorageClass owner already owns. StorageClass is almost entirely immutable
// (only allowVolumeExpansion and metadata can change), so drift on immutable
// fields is a terminal error naming the one action that fixes it.
func applyOwnedStorageClass(ctx context.Context, c client.Client, owner client.Object, ownerKind string, desired *storagev1.StorageClass) error {
	desired.OwnerReferences = []metav1.OwnerReference{controllerRef(owner, ownerKind)}

	var existing storagev1.StorageClass
	err := c.Get(ctx, types.NamespacedName{Name: desired.Name}, &existing)
	if apierrors.IsNotFound(err) {
		if err := c.Create(ctx, desired); err != nil {
			return fmt.Errorf("create StorageClass: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get StorageClass: %w", err)
	}

	if !ownedBy(existing.OwnerReferences, ownerKind, owner) {
		return notOursError(ownerKind, "StorageClass", desired.Name)
	}

	if drift := immutableStorageClassDrift(&existing, desired); len(drift) > 0 {
		//nolint:wrapcheck // TerminalError is a wrapper; wrapping it again adds nothing
		return reconcile.TerminalError(fmt.Errorf(
			"StorageClass %q differs from the desired spec on immutable field(s) %s; "+
				"Kubernetes does not allow updating these. Delete the StorageClass to have it recreated "+
				"(existing PersistentVolumes keep working; only new provisioning uses it)",
			desired.Name, strings.Join(drift, ", ")))
	}

	before := existing.DeepCopy()
	existing.AllowVolumeExpansion = desired.AllowVolumeExpansion
	// Upserts our controller ref and keeps any other party's ownerRef, unlike
	// assigning desired.OwnerReferences wholesale.
	if err := controllerutil.SetControllerReference(owner, &existing, c.Scheme()); err != nil {
		return fmt.Errorf("set StorageClass controller reference: %w", err)
	}
	// Skip the identical write: Disk events fan out here on every reconcile.
	if equality.Semantic.DeepEqual(before, &existing) {
		return nil
	}
	if err := c.Update(ctx, &existing); err != nil {
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
	if !maps.Equal(existing.Parameters, desired.Parameters) {
		drift = append(drift, "parameters")
	}
	if !ptr.Equal(existing.ReclaimPolicy, desired.ReclaimPolicy) {
		drift = append(drift, "reclaimPolicy")
	}
	if !ptr.Equal(existing.VolumeBindingMode, desired.VolumeBindingMode) {
		drift = append(drift, "volumeBindingMode")
	}
	sort.Strings(drift)
	return drift
}
