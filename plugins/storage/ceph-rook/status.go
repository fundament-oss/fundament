package main

import (
	"context"
	"fmt"
	"slices"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

// statusView is what the shared writer manages on a status struct;
// *ConsumerStatus and *StoragePoolStatus implement it.
type statusView interface {
	SetObservedGeneration(int64)
	ConditionsRef() *[]metav1.Condition
	SetDegraded(message string)
}

// statusWriter persists one kind's status struct via the status subresource.
// It owns observedGeneration and the Ready condition; callers build the
// observable fields.
type statusWriter[O client.Object, S any, PS interface {
	*S
	statusView
}] struct {
	client    client.Client
	kind      string
	newObject func() O
	statusOf  func(O) *S
}

// write persists status, refetching first so a stale resourceVersion from
// earlier in the reconcile doesn't cause a conflict.
//
// ready is upserted as the Ready condition. Conditions and observedGeneration
// are taken from the refetched object rather than from the caller's status
// value: the caller builds the observable fields, this decides what generation
// they describe.
func (w statusWriter[O, S, PS]) write(ctx context.Context, name string, status *S, ready *metav1.Condition) error {
	current := w.newObject()
	if err := w.client.Get(ctx, types.NamespacedName{Name: name}, current); err != nil {
		if apierrors.IsNotFound(err) {
			return nil // deleted mid-reconcile; nothing to record
		}
		return fmt.Errorf("get %s %s for status update: %w", w.kind, name, err)
	}

	PS(status).SetObservedGeneration(current.GetGeneration())
	// Carry the existing conditions so SetStatusCondition can preserve
	// lastTransitionTime for a condition whose status has not flipped. Cloned,
	// not aliased: SetStatusCondition mutates the existing element through a
	// pointer into the backing array, so sharing it with current would mutate
	// both sides and the DeepEqual below could never see a condition change.
	conditions := PS(status).ConditionsRef()
	*conditions = slices.Clone(*PS(w.statusOf(current)).ConditionsRef())
	ready.Type = v1alpha1.ConditionReady
	ready.ObservedGeneration = current.GetGeneration()
	meta.SetStatusCondition(conditions, *ready)

	// Every Disk event in the cluster fans out to a reconcile of every object
	// of this kind, and the status is identical almost every time. The API
	// server would no-op the write, but not before it has been serialised and
	// sent. Compared semantically rather than with ==, which stopped compiling
	// once Conditions was added and would silently compare slice headers if it
	// ever did.
	if equality.Semantic.DeepEqual(*w.statusOf(current), *status) {
		return nil
	}
	*w.statusOf(current) = *status
	if err := w.client.Status().Update(ctx, current); err != nil {
		return fmt.Errorf("update %s status %s: %w", w.kind, name, err)
	}
	return nil
}

// setDegraded records a reconcile failure on obj. Status write failures are
// logged rather than returned: the caller is already returning the real error,
// and masking it with a status-write error would hide the cause.
func (w statusWriter[O, S, PS]) setDegraded(ctx context.Context, obj O, cause error) {
	status := *w.statusOf(obj)
	PS(&status).SetDegraded(cause.Error())
	ready := metav1.Condition{
		Status:  metav1.ConditionFalse,
		Reason:  v1alpha1.ReasonReconcileError,
		Message: cause.Error(),
	}
	if err := w.write(ctx, obj.GetName(), &status, &ready); err != nil {
		log.FromContext(ctx).Error(err, "could not record degraded status", "kind", w.kind, "name", obj.GetName())
	}
}
