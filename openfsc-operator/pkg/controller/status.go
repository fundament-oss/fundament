package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openfscv1 "github.com/fundament-oss/fundament/openfsc-operator/pkg/api/v1"
)

// gatewayRequeue is the steady-state poll interval for observing registrations.
const gatewayRequeue = time.Minute

// setSynced sets the shared "Synced" condition on a gateway status.
func setSynced(st *openfscv1.Status, status metav1.ConditionStatus, gen int64, reason, msg string) {
	meta.SetStatusCondition(&st.Conditions, metav1.Condition{
		Type: "Synced", Status: status, Reason: reason, Message: msg, ObservedGeneration: gen,
	})
}

func setActive(st *openfscv1.Status, gen int64, msg string) {
	now := metav1.Now()
	st.Phase = openfscv1.PhaseActive
	st.Message = msg
	st.ObservedGeneration = gen
	st.LastSyncedTime = &now
	setSynced(st, metav1.ConditionTrue, gen, "Synced", msg)
}

func setPending(st *openfscv1.Status, gen int64, reason, msg string) {
	st.Phase = openfscv1.PhasePending
	st.Message = msg
	st.ObservedGeneration = gen
	setSynced(st, metav1.ConditionFalse, gen, reason, msg)
}

func setError(st *openfscv1.Status, gen int64, msg string) {
	st.Phase = openfscv1.PhaseError
	st.Message = msg
	st.ObservedGeneration = gen
	setSynced(st, metav1.ConditionFalse, gen, "ReconcileError", msg)
}

// reflectRegistration writes an Active/Pending status from a boolean, skipping
// the write once the state has already been reported for the current generation.
func reflectRegistration(ctx context.Context, c client.Client, obj client.Object, st *openfscv1.Status, gen int64, present bool, activeMsg, pendingMsg string) (ctrl.Result, error) {
	// Compare the Synced condition reason, not just its True/False status: an
	// earlier Error also leaves Synced=False, so checking only the status would
	// treat a now-resolved NotRegistered as "already reported" and freeze the CR
	// at Error. setActive uses reason "Synced"; setPending uses "NotRegistered".
	want := "NotRegistered"
	if present {
		want = "Synced"
	}
	cond := meta.FindStatusCondition(st.Conditions, "Synced")
	if st.ObservedGeneration == gen && cond != nil && cond.Reason == want {
		return ctrl.Result{RequeueAfter: gatewayRequeue}, nil
	}
	if present {
		setActive(st, gen, activeMsg)
	} else {
		setPending(st, gen, "NotRegistered", pendingMsg)
	}
	if err := c.Status().Update(ctx, obj); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}
	return ctrl.Result{RequeueAfter: gatewayRequeue}, nil
}

// reflectError records a reconcile error, skipping the write when the same error
// is already recorded for the current generation.
func reflectError(ctx context.Context, c client.Client, obj client.Object, st *openfscv1.Status, gen int64, msg string) (ctrl.Result, error) {
	if st.ObservedGeneration == gen && st.Phase == openfscv1.PhaseError && st.Message == msg {
		return ctrl.Result{RequeueAfter: gatewayRequeue}, nil
	}
	setError(st, gen, msg)
	if err := c.Status().Update(ctx, obj); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}
	return ctrl.Result{RequeueAfter: gatewayRequeue}, nil
}

// reportNotConfigured records that the Controller Administration API client is
// not available yet, so the gateway's registration cannot be observed.
func reportNotConfigured(ctx context.Context, c client.Client, obj client.Object, st *openfscv1.Status, gen int64) (ctrl.Result, error) {
	const reason = "NotConfigured"
	cond := meta.FindStatusCondition(st.Conditions, "Synced")
	if st.ObservedGeneration == gen && cond != nil && cond.Reason == reason {
		return ctrl.Result{RequeueAfter: gatewayRequeue}, nil
	}
	setPending(st, gen, reason, "OpenFSC Controller Administration API not configured")
	if err := c.Status().Update(ctx, obj); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}
	return ctrl.Result{RequeueAfter: gatewayRequeue}, nil
}

// reportPending records a not-yet-ready status with a custom reason, skipping the
// write when the same reason is already recorded for the current generation.
func reportPending(ctx context.Context, c client.Client, obj client.Object, st *openfscv1.Status, gen int64, reason, msg string, requeue time.Duration) (ctrl.Result, error) {
	cond := meta.FindStatusCondition(st.Conditions, "Synced")
	if st.ObservedGeneration != gen || cond == nil || cond.Reason != reason {
		setPending(st, gen, reason, msg)
		if err := c.Status().Update(ctx, obj); err != nil {
			return ctrl.Result{}, fmt.Errorf("update status: %w", err)
		}
	}
	return ctrl.Result{RequeueAfter: requeue}, nil
}

func deploymentAvailable(deploy *appsv1.Deployment) bool {
	for _, cond := range deploy.Status.Conditions {
		if cond.Type == appsv1.DeploymentAvailable {
			return cond.Status == "True"
		}
	}
	return false
}
