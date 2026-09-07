package main

import (
	"context"
	"fmt"
	"slices"
	"time"

	storagev1 "k8s.io/api/storage/v1"
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

// provisioningRequeue is how often a BlockStorage that is not yet Ready
// re-checks its CephBlockPool, rather than waiting out the manager's ~10h
// resync.
const provisioningRequeue = 30 * time.Second

// BlockStorageReconciler reconciles BlockStorage objects into a CephBlockPool
// and an RBD StorageClass over the shared OSD set. It never writes CephCluster
// spec.storage.nodes; StoragePoolReconciler owns that.
type BlockStorageReconciler struct {
	Client           client.Client
	ClusterNamespace string
	RookNamespace    string
}

func (r *BlockStorageReconciler) SetupWithManager(mgr manager.Manager) error {
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.BlockStorage{}).
		Owns(&storagev1.StorageClass{}).
		Owns(cephBlockPoolStub()).
		Watches(&v1alpha1.Disk{}, handler.EnqueueRequestsFromMapFunc(r.toAllBlockStorages)).
		Watches(&v1alpha1.StoragePool{}, handler.EnqueueRequestsFromMapFunc(r.toAllBlockStorages)).
		Complete(r); err != nil {
		return fmt.Errorf("register BlockStorage controller: %w", err)
	}
	return nil
}

// toAllBlockStorages maps Disk and StoragePool events to every BlockStorage:
// either can change the contributing node count and therefore replication.
func (r *BlockStorageReconciler) toAllBlockStorages(ctx context.Context, _ client.Object) []reconcile.Request {
	var list v1alpha1.BlockStorageList
	if err := r.Client.List(ctx, &list); err != nil {
		return nil
	}
	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Name: list.Items[i].Name}})
	}
	return reqs
}

func (r *BlockStorageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var bs v1alpha1.BlockStorage
	if err := r.Client.Get(ctx, req.NamespacedName, &bs); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil // owner refs GC the derived objects
		}
		return ctrl.Result{}, fmt.Errorf("get BlockStorage %s: %w", req.Name, err)
	}
	if !bs.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	result, err := r.reconcile(ctx, &bs)
	if err != nil {
		r.setDegraded(ctx, &bs, err)
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *BlockStorageReconciler) reconcile(ctx context.Context, bs *v1alpha1.BlockStorage) (ctrl.Result, error) {
	union, err := diskUnion(ctx, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("compute disk union: %w", err)
	}

	// Without OSDs a CephBlockPool never provisions and its StorageClass leaves
	// every PVC Pending. Report instead of creating; the StoragePool watch wakes
	// us when capacity appears.
	if len(union) == 0 {
		status := bs.Status
		status.Phase = v1alpha1.PhaseDegraded
		status.StorageClassName, status.FailureDomain = "", ""
		status.Replicas = 0
		status.Message = "no OSDs: create a StoragePool with disks first"
		return ctrl.Result{}, r.writeStatus(ctx, bs, &status, &metav1.Condition{
			Status:  metav1.ConditionFalse,
			Reason:  v1alpha1.ReasonNoOSDs,
			Message: status.Message,
		})
	}

	replicas, domain, msg := ComputeReplication(bs.Spec.Replication, DistinctNodeCount(union))
	derived := DerivedName(bs.Name)

	cbp := RenderCephBlockPool(r.ClusterNamespace, derived, replicas, domain)
	if err := applyOwnedRookObject(ctx, r.Client, controllerRef(bs, "BlockStorage"), "BlockStorage", cbp); err != nil {
		return ctrl.Result{}, fmt.Errorf("apply CephBlockPool %s: %w", derived, err)
	}

	desired := RenderStorageClass(derived, r.ClusterNamespace, derived, r.RookNamespace)
	if err := applyOwnedStorageClass(ctx, r.Client, bs, "BlockStorage", desired); err != nil {
		return ctrl.Result{}, fmt.Errorf("apply StorageClass %s: %w", derived, err)
	}

	phase, err := rookObjectPhase(ctx, r.Client, r.ClusterNamespace, derived, "CephBlockPool")
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("read CephBlockPool %s phase: %w", derived, err)
	}

	ready := metav1.Condition{
		Status:  metav1.ConditionFalse,
		Reason:  v1alpha1.ReasonProvisioning,
		Message: "waiting for CephBlockPool " + derived + " to report Ready",
	}
	if phase == v1alpha1.PhaseReady {
		ready.Status = metav1.ConditionTrue
		ready.Reason = v1alpha1.ReasonReady
		ready.Message = msg
		if ready.Message == "" {
			ready.Message = "CephBlockPool " + derived + " is Ready"
		}
	}

	if err := r.writeStatus(ctx, bs, &v1alpha1.BlockStorageStatus{
		Phase:            phase,
		StorageClassName: derived,
		Replicas:         replicas,
		FailureDomain:    domain,
		Message:          msg,
	}, &ready); err != nil {
		return ctrl.Result{}, err
	}

	if phase != v1alpha1.PhaseReady {
		return ctrl.Result{RequeueAfter: provisioningRequeue}, nil
	}
	return ctrl.Result{}, nil
}

// writeStatus persists status via the status subresource, refetching first so a
// stale resourceVersion from earlier in the reconcile doesn't cause a conflict.
//
// ready is upserted as the BlockStorage's Ready condition. Conditions and
// observedGeneration are taken from the refetched object rather than from the
// caller's status value: the caller builds the observable fields, this decides
// what generation they describe.
func (r *BlockStorageReconciler) writeStatus(ctx context.Context, bs *v1alpha1.BlockStorage, status *v1alpha1.BlockStorageStatus, ready *metav1.Condition) error {
	var current v1alpha1.BlockStorage
	if err := r.Client.Get(ctx, types.NamespacedName{Name: bs.Name}, &current); err != nil {
		if apierrors.IsNotFound(err) {
			return nil // deleted mid-reconcile; nothing to record
		}
		return fmt.Errorf("get BlockStorage %s for status update: %w", bs.Name, err)
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

	// Every Disk event in the cluster fans out to a reconcile of every
	// BlockStorage, and the status is identical almost every time. The API
	// server would no-op the write, but not before it has been serialised and
	// sent. Compared semantically rather than with ==, which stopped compiling
	// once Conditions was added and would silently compare slice headers if it
	// ever did.
	if equality.Semantic.DeepEqual(current.Status, *status) {
		return nil
	}
	current.Status = *status
	if err := r.Client.Status().Update(ctx, &current); err != nil {
		return fmt.Errorf("update BlockStorage status %s: %w", bs.Name, err)
	}
	return nil
}

// setDegraded records a reconcile failure on the BlockStorage. Status write
// failures are logged rather than returned: the caller is already returning
// the real error, and masking it with a status-write error would hide the
// cause.
func (r *BlockStorageReconciler) setDegraded(ctx context.Context, bs *v1alpha1.BlockStorage, cause error) {
	status := bs.Status
	status.Phase = v1alpha1.PhaseDegraded
	status.Message = cause.Error()
	ready := metav1.Condition{
		Status:  metav1.ConditionFalse,
		Reason:  v1alpha1.ReasonReconcileError,
		Message: cause.Error(),
	}
	if err := r.writeStatus(ctx, bs, &status, &ready); err != nil {
		log.FromContext(ctx).Error(err, "could not record degraded status", "blockStorage", bs.Name)
	}
}

// cephBlockPoolStub is an empty CephBlockPool carrying only its GVK, which is
// all Owns needs to start an informer for a type absent from the scheme.
func cephBlockPoolStub() *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetAPIVersion(cephAPIVersion)
	u.SetKind("CephBlockPool")
	return u
}

// applyOwnedRookObject creates or updates an unstructured Rook object and
// sets ownerRef as its controller owner. An existing object the owner does
// not already own is left alone (Rook keeps its own objects in this
// namespace; adopting one would delete it with the owner).
func applyOwnedRookObject(ctx context.Context, c client.Client, ownerRef metav1.OwnerReference, ownerKind string, desired *unstructured.Unstructured) error {
	desired.SetOwnerReferences([]metav1.OwnerReference{ownerRef})
	kind := desired.GetKind()

	existing := &unstructured.Unstructured{}
	existing.SetAPIVersion(cephAPIVersion)
	existing.SetKind(kind)
	err := c.Get(ctx, types.NamespacedName{Namespace: desired.GetNamespace(), Name: desired.GetName()}, existing)
	if apierrors.IsNotFound(err) {
		if err := c.Create(ctx, desired); err != nil {
			return fmt.Errorf("create %s: %w", kind, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get %s: %w", kind, err)
	}

	owned := false
	for _, ref := range existing.GetOwnerReferences() {
		if ref.Controller != nil && *ref.Controller &&
			ref.Kind == ownerRef.Kind && ref.Name == ownerRef.Name && ref.UID == ownerRef.UID {
			owned = true
			break
		}
	}
	if !owned {
		return notOursError(ownerKind, kind, desired.GetName())
	}

	// Update spec and owner refs while preserving the existing resource version.
	existing.Object["spec"] = desired.Object["spec"]
	existing.SetOwnerReferences(desired.GetOwnerReferences())
	if err := c.Update(ctx, existing); err != nil {
		return fmt.Errorf("update %s: %w", kind, err)
	}
	return nil
}

// rookObjectPhase reads an unstructured Rook object's status.phase to derive
// the owning resource's Phase. An object that does not exist yet is
// Provisioning; any other error is returned rather than reported as
// Provisioning, so an RBAC or API failure surfaces instead of looping silently.
func rookObjectPhase(ctx context.Context, c client.Client, namespace, name, kind string) (string, error) {
	existing := &unstructured.Unstructured{}
	existing.SetAPIVersion(cephAPIVersion)
	existing.SetKind(kind)
	err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, existing)
	if apierrors.IsNotFound(err) {
		return v1alpha1.PhaseProvisioning, nil
	}
	if err != nil {
		return "", fmt.Errorf("get %s: %w", kind, err)
	}
	phase, _, _ := unstructured.NestedString(existing.Object, "status", "phase")
	if phase == v1alpha1.PhaseReady {
		return v1alpha1.PhaseReady, nil
	}
	return v1alpha1.PhaseProvisioning, nil
}
