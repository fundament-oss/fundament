package main

import (
	"context"
	"fmt"
	"time"

	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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

// provisioningRequeue is how often a consumer that is not yet Ready re-checks
// its derived Rook object, rather than waiting out the manager's ~10h resync.
const provisioningRequeue = 30 * time.Second

// rookPhaseFailure is Rook's terminal status.phase (rook.io ConditionFailure);
// its other phases (Progressing, Connecting, ...) all map to Provisioning.
const rookPhaseFailure = "Failure"

// consumer is a cluster-scoped kind that turns the shared OSD set into a
// StorageClass (BlockStorage, FileStorage). It derives exactly one Rook object
// and one StorageClass, and brings no disks of its own.
type consumer interface {
	client.Object
	ConsumerStatus() *v1alpha1.ConsumerStatus
	ReplicationSpec() string
}

// ConsumerReconciler reconciles one consumer kind into its derived Rook object
// and StorageClass over the shared OSD set. It never writes CephCluster
// spec.storage.nodes; StoragePoolReconciler owns that. The kind-specific
// pieces (names, renderers) come in via the New*Reconciler constructors.
type ConsumerReconciler[T consumer] struct {
	Client           client.Client
	ClusterNamespace string
	RookNamespace    string

	kind               string // e.g. "BlockStorage"
	rookKind           string // e.g. "CephBlockPool"
	derivedName        func(string) string
	newObject          func() T
	newList            func() client.ObjectList
	renderRook         func(obj T, namespace, name string, replicas int, domain string) *unstructured.Unstructured
	renderStorageClass func(name, clusterNamespace, poolOrFSName, rookNamespace string) *storagev1.StorageClass
}

// NewBlockStorageReconciler reconciles BlockStorage into a CephBlockPool and
// an RBD StorageClass.
func NewBlockStorageReconciler(c client.Client, clusterNamespace, rookNamespace string) *ConsumerReconciler[*v1alpha1.BlockStorage] {
	return &ConsumerReconciler[*v1alpha1.BlockStorage]{
		Client:           c,
		ClusterNamespace: clusterNamespace,
		RookNamespace:    rookNamespace,
		kind:             "BlockStorage",
		rookKind:         "CephBlockPool",
		derivedName:      DerivedName,
		newObject:        func() *v1alpha1.BlockStorage { return &v1alpha1.BlockStorage{} },
		newList:          func() client.ObjectList { return &v1alpha1.BlockStorageList{} },
		renderRook: func(_ *v1alpha1.BlockStorage, namespace, name string, replicas int, domain string) *unstructured.Unstructured {
			return RenderCephBlockPool(namespace, name, replicas, domain)
		},
		renderStorageClass: RenderStorageClass,
	}
}

// NewFileStorageReconciler reconciles FileStorage into a CephFilesystem and a
// CephFS StorageClass.
func NewFileStorageReconciler(c client.Client, clusterNamespace, rookNamespace string) *ConsumerReconciler[*v1alpha1.FileStorage] {
	return &ConsumerReconciler[*v1alpha1.FileStorage]{
		Client:           c,
		ClusterNamespace: clusterNamespace,
		RookNamespace:    rookNamespace,
		kind:             "FileStorage",
		rookKind:         "CephFilesystem",
		derivedName:      FilesystemDerivedName,
		newObject:        func() *v1alpha1.FileStorage { return &v1alpha1.FileStorage{} },
		newList:          func() client.ObjectList { return &v1alpha1.FileStorageList{} },
		renderRook: func(fs *v1alpha1.FileStorage, namespace, name string, replicas int, domain string) *unstructured.Unstructured {
			return RenderCephFilesystem(namespace, name, replicas, domain, int64(fs.Spec.MetadataServers))
		},
		renderStorageClass: RenderCephFSStorageClass,
	}
}

func (r *ConsumerReconciler[T]) SetupWithManager(mgr manager.Manager) error {
	if err := ctrl.NewControllerManagedBy(mgr).
		For(r.newObject()).
		Owns(&storagev1.StorageClass{}).
		Owns(rookStub(r.rookKind)).
		// No predicate on Disk: its changes arrive via the status subresource,
		// which a generation filter would drop. StoragePool matters only through
		// spec.disks, and the predicate keeps every pool status write from
		// re-enqueueing every consumer.
		Watches(&v1alpha1.Disk{}, handler.EnqueueRequestsFromMapFunc(r.toAllConsumers)).
		Watches(&v1alpha1.StoragePool{}, handler.EnqueueRequestsFromMapFunc(r.toAllConsumers),
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		// Owns only sees objects carrying our ownerRef. These watches cover
		// foreign same-named objects, whose removal is notOursError's
		// documented recovery and would otherwise produce no event.
		Watches(&storagev1.StorageClass{}, handler.EnqueueRequestsFromMapFunc(r.toSameNamedConsumer)).
		Watches(rookStub(r.rookKind), handler.EnqueueRequestsFromMapFunc(r.toSameNamedConsumer)).
		Complete(r); err != nil {
		return fmt.Errorf("register %s controller: %w", r.kind, err)
	}
	return nil
}

// toAllConsumers maps Disk and StoragePool events to every consumer of this
// kind: either can change the contributing node count and therefore
// replication.
func (r *ConsumerReconciler[T]) toAllConsumers(ctx context.Context, _ client.Object) []reconcile.Request {
	list := r.newList()
	if err := r.Client.List(ctx, list); err != nil {
		return nil
	}
	items, err := meta.ExtractList(list)
	if err != nil {
		return nil
	}
	reqs := make([]reconcile.Request, 0, len(items))
	for _, item := range items {
		obj := item.(client.Object)
		reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Name: obj.GetName()}})
	}
	return reqs
}

// toSameNamedConsumer maps an event on a derived-named object (StorageClass or
// the Rook kind) to the consumer whose derived name matches, whether or not
// the object carries our ownerRef.
func (r *ConsumerReconciler[T]) toSameNamedConsumer(ctx context.Context, obj client.Object) []reconcile.Request {
	for _, req := range r.toAllConsumers(ctx, nil) {
		if r.derivedName(req.Name) == obj.GetName() {
			return []reconcile.Request{req}
		}
	}
	return nil
}

func (r *ConsumerReconciler[T]) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	obj := r.newObject()
	if err := r.Client.Get(ctx, req.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil // owner refs GC the derived objects
		}
		return ctrl.Result{}, fmt.Errorf("get %s %s: %w", r.kind, req.Name, err)
	}
	if !obj.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	result, err := r.reconcile(ctx, obj)
	if err != nil {
		r.setDegraded(ctx, obj, err)
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *ConsumerReconciler[T]) reconcile(ctx context.Context, obj T) (ctrl.Result, error) {
	union, err := diskUnion(ctx, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("compute disk union: %w", err)
	}

	// Without OSDs the derived Rook object never provisions and its
	// StorageClass leaves every PVC Pending. Report instead of creating; the
	// StoragePool watch wakes us when capacity appears.
	if len(union) == 0 {
		status := *obj.ConsumerStatus()
		status.Phase = v1alpha1.PhaseDegraded
		status.StorageClassName, status.FailureDomain = "", ""
		status.Replicas = 0
		status.Message = "no OSDs: create a StoragePool with disks first"
		return ctrl.Result{}, r.writeStatus(ctx, obj, &status, &metav1.Condition{
			Status:  metav1.ConditionFalse,
			Reason:  v1alpha1.ReasonNoOSDs,
			Message: status.Message,
		})
	}

	replicas, domain, msg := ComputeReplication(obj.ReplicationSpec(), DistinctNodeCount(union))
	derived := r.derivedName(obj.GetName())

	rook, err := applyOwnedRookObject(ctx, r.Client, obj, r.kind,
		r.renderRook(obj, r.ClusterNamespace, derived, replicas, domain))
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("apply %s %s: %w", r.rookKind, derived, err)
	}

	desired := r.renderStorageClass(derived, r.ClusterNamespace, derived, r.RookNamespace)
	if err := applyOwnedStorageClass(ctx, r.Client, obj, r.kind, desired); err != nil {
		return ctrl.Result{}, fmt.Errorf("apply StorageClass %s: %w", derived, err)
	}

	phase := rookObjectPhase(rook)

	ready := metav1.Condition{
		Status:  metav1.ConditionFalse,
		Reason:  v1alpha1.ReasonProvisioning,
		Message: "waiting for " + r.rookKind + " " + derived + " to report Ready",
	}
	switch phase {
	case v1alpha1.PhaseReady:
		ready.Status = metav1.ConditionTrue
		ready.Reason = v1alpha1.ReasonReady
		ready.Message = msg
		if ready.Message == "" {
			ready.Message = r.rookKind + " " + derived + " is Ready"
		}
	case v1alpha1.PhaseDegraded:
		ready.Reason = v1alpha1.ReasonRookFailure
		ready.Message = r.rookKind + " " + derived + " reports Failure; check the Rook operator logs"
		msg = ready.Message
	}

	if err := r.writeStatus(ctx, obj, &v1alpha1.ConsumerStatus{
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

// statusWriter builds the shared writer (see status.go) for this consumer kind.
func (r *ConsumerReconciler[T]) statusWriter() statusWriter[T, v1alpha1.ConsumerStatus, *v1alpha1.ConsumerStatus] {
	return statusWriter[T, v1alpha1.ConsumerStatus, *v1alpha1.ConsumerStatus]{
		client:    r.Client,
		kind:      r.kind,
		newObject: r.newObject,
		statusOf:  func(o T) *v1alpha1.ConsumerStatus { return o.ConsumerStatus() },
	}
}

func (r *ConsumerReconciler[T]) writeStatus(ctx context.Context, obj T, status *v1alpha1.ConsumerStatus, ready *metav1.Condition) error {
	return r.statusWriter().write(ctx, obj.GetName(), status, ready)
}

func (r *ConsumerReconciler[T]) setDegraded(ctx context.Context, obj T, cause error) {
	r.statusWriter().setDegraded(ctx, obj, cause)
}

// rookStub is an empty Rook object carrying only its GVK, which is all Owns
// needs to start an informer for a type absent from the scheme.
func rookStub(kind string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetAPIVersion(cephAPIVersion)
	u.SetKind(kind)
	return u
}

// applyOwnedRookObject creates or updates an unstructured Rook object and
// sets ownerRef as its controller owner. An existing object the owner does
// not already own is left alone (Rook keeps its own objects in this
// namespace; adopting one would delete it with the owner).
//
// It returns the live object (with server-populated status), so the caller
// reads the phase without a second Get. An identical object is not rewritten,
// for the same fan-out reason as writeStatus.
func applyOwnedRookObject(ctx context.Context, c client.Client, owner client.Object, ownerKind string, desired *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	desired.SetOwnerReferences([]metav1.OwnerReference{controllerRef(owner, ownerKind)})
	kind := desired.GetKind()

	existing := rookStub(kind)
	err := c.Get(ctx, types.NamespacedName{Namespace: desired.GetNamespace(), Name: desired.GetName()}, existing)
	if apierrors.IsNotFound(err) {
		if err := c.Create(ctx, desired); err != nil {
			return nil, fmt.Errorf("create %s: %w", kind, err)
		}
		return desired, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", kind, err)
	}

	if !ownedBy(existing.GetOwnerReferences(), ownerKind, owner) {
		return nil, notOursError(ownerKind, kind, desired.GetName())
	}

	if equality.Semantic.DeepEqual(existing.Object["spec"], desired.Object["spec"]) {
		return existing, nil
	}

	// Update only spec, keeping the existing owner references: ownedBy proved
	// our controller ref is present, and assigning desired's list wholesale
	// would drop a foreign non-controller ref (see applyOwnedStorageClass).
	existing.Object["spec"] = desired.Object["spec"]
	if err := c.Update(ctx, existing); err != nil {
		return nil, fmt.Errorf("update %s: %w", kind, err)
	}
	return existing, nil
}

// rookObjectPhase derives the owner's Phase from a Rook object's status.phase.
// A just-created object has no status yet: Provisioning.
func rookObjectPhase(obj *unstructured.Unstructured) string {
	phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
	switch phase {
	case v1alpha1.PhaseReady:
		return v1alpha1.PhaseReady
	case rookPhaseFailure:
		// A dead end (bad spec, unschedulable daemons), not progress: reporting
		// it as Provisioning would hide the failure forever.
		return v1alpha1.PhaseDegraded
	default:
		return v1alpha1.PhaseProvisioning
	}
}
