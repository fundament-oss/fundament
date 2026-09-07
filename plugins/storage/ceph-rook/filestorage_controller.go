package main

import (
	"context"
	"fmt"
	"slices"

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

// FileStorageReconciler reconciles FileStorage objects into a CephFilesystem
// and a CephFS StorageClass over the shared OSD set. It never writes
// CephCluster spec.storage.nodes; StoragePoolReconciler owns that.
type FileStorageReconciler struct {
	Client           client.Client
	ClusterNamespace string
	RookNamespace    string
}

func (r *FileStorageReconciler) SetupWithManager(mgr manager.Manager) error {
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.FileStorage{}).
		Owns(&storagev1.StorageClass{}).
		Owns(cephFilesystemStub()).
		Watches(&v1alpha1.Disk{}, handler.EnqueueRequestsFromMapFunc(r.toAllFileStorages)).
		Watches(&v1alpha1.StoragePool{}, handler.EnqueueRequestsFromMapFunc(r.toAllFileStorages)).
		Complete(r); err != nil {
		return fmt.Errorf("register FileStorage controller: %w", err)
	}
	return nil
}

// toAllFileStorages maps Disk and StoragePool events to every FileStorage:
// either can change the contributing node count and therefore replication.
func (r *FileStorageReconciler) toAllFileStorages(ctx context.Context, _ client.Object) []reconcile.Request {
	var list v1alpha1.FileStorageList
	if err := r.Client.List(ctx, &list); err != nil {
		return nil
	}
	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Name: list.Items[i].Name}})
	}
	return reqs
}

func (r *FileStorageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var fs v1alpha1.FileStorage
	if err := r.Client.Get(ctx, req.NamespacedName, &fs); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil // owner refs GC the derived objects
		}
		return ctrl.Result{}, fmt.Errorf("get FileStorage %s: %w", req.Name, err)
	}
	if !fs.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	result, err := r.reconcile(ctx, &fs)
	if err != nil {
		r.setDegraded(ctx, &fs, err)
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *FileStorageReconciler) reconcile(ctx context.Context, fs *v1alpha1.FileStorage) (ctrl.Result, error) {
	union, err := diskUnion(ctx, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("compute disk union: %w", err)
	}

	// Without OSDs a CephFilesystem never provisions and its StorageClass leaves
	// every PVC Pending. Report instead of creating; the StoragePool watch wakes
	// us when capacity appears.
	if len(union) == 0 {
		status := fs.Status
		status.Phase = v1alpha1.PhaseDegraded
		status.StorageClassName, status.FailureDomain = "", ""
		status.Replicas = 0
		status.Message = "no OSDs: create a StoragePool with disks first"
		return ctrl.Result{}, r.writeStatus(ctx, fs, &status, &metav1.Condition{
			Status:  metav1.ConditionFalse,
			Reason:  v1alpha1.ReasonNoOSDs,
			Message: status.Message,
		})
	}

	replicas, domain, msg := ComputeReplication(fs.Spec.Replication, DistinctNodeCount(union))
	derived := FilesystemDerivedName(fs.Name)

	cfs := RenderCephFilesystem(r.ClusterNamespace, derived, replicas, domain, int64(fs.Spec.MetadataServers))
	if err := applyOwnedRookObject(ctx, r.Client, controllerRef(fs, "FileStorage"), "FileStorage", cfs); err != nil {
		return ctrl.Result{}, fmt.Errorf("apply CephFilesystem %s: %w", derived, err)
	}

	desired := RenderCephFSStorageClass(derived, r.ClusterNamespace, derived, r.RookNamespace)
	if err := applyOwnedStorageClass(ctx, r.Client, fs, "FileStorage", desired); err != nil {
		return ctrl.Result{}, fmt.Errorf("apply StorageClass %s: %w", derived, err)
	}

	phase, err := rookObjectPhase(ctx, r.Client, r.ClusterNamespace, derived, "CephFilesystem")
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("read CephFilesystem %s phase: %w", derived, err)
	}

	ready := metav1.Condition{
		Status:  metav1.ConditionFalse,
		Reason:  v1alpha1.ReasonProvisioning,
		Message: "waiting for CephFilesystem " + derived + " to report Ready",
	}
	if phase == v1alpha1.PhaseReady {
		ready.Status = metav1.ConditionTrue
		ready.Reason = v1alpha1.ReasonReady
		ready.Message = msg
		if ready.Message == "" {
			ready.Message = "CephFilesystem " + derived + " is Ready"
		}
	}

	if err := r.writeStatus(ctx, fs, &v1alpha1.FileStorageStatus{
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
// ready is upserted as the FileStorage's Ready condition. Conditions and
// observedGeneration are taken from the refetched object rather than from the
// caller's status value: the caller builds the observable fields, this decides
// what generation they describe.
func (r *FileStorageReconciler) writeStatus(ctx context.Context, fs *v1alpha1.FileStorage, status *v1alpha1.FileStorageStatus, ready *metav1.Condition) error {
	var current v1alpha1.FileStorage
	if err := r.Client.Get(ctx, types.NamespacedName{Name: fs.Name}, &current); err != nil {
		if apierrors.IsNotFound(err) {
			return nil // deleted mid-reconcile; nothing to record
		}
		return fmt.Errorf("get FileStorage %s for status update: %w", fs.Name, err)
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
	// FileStorage, and the status is identical almost every time. The API
	// server would no-op the write, but not before it has been serialised and
	// sent. Compared semantically rather than with ==, which stopped compiling
	// once Conditions was added and would silently compare slice headers if it
	// ever did.
	if equality.Semantic.DeepEqual(current.Status, *status) {
		return nil
	}
	current.Status = *status
	if err := r.Client.Status().Update(ctx, &current); err != nil {
		return fmt.Errorf("update FileStorage status %s: %w", fs.Name, err)
	}
	return nil
}

// setDegraded records a reconcile failure on the FileStorage. Status write
// failures are logged rather than returned: the caller is already returning
// the real error, and masking it with a status-write error would hide the
// cause.
func (r *FileStorageReconciler) setDegraded(ctx context.Context, fs *v1alpha1.FileStorage, cause error) {
	status := fs.Status
	status.Phase = v1alpha1.PhaseDegraded
	status.Message = cause.Error()
	ready := metav1.Condition{
		Status:  metav1.ConditionFalse,
		Reason:  v1alpha1.ReasonReconcileError,
		Message: cause.Error(),
	}
	if err := r.writeStatus(ctx, fs, &status, &ready); err != nil {
		log.FromContext(ctx).Error(err, "could not record degraded status", "fileStorage", fs.Name)
	}
}

// cephFilesystemStub is an empty CephFilesystem carrying only its GVK, which is
// all Owns needs to start an informer for a type absent from the scheme.
func cephFilesystemStub() *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetAPIVersion(cephAPIVersion)
	u.SetKind("CephFilesystem")
	return u
}
