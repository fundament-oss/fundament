package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openfscv1 "github.com/fundament-oss/fundament/openfsc-operator/pkg/api/v1"
)

// PeerReconciler reconciles Peer resources. For the directory peer deployed by
// a Directory resource, "Active" means the OpenFSC Manager and Controller
// Deployments are Available — a dependency-free readiness signal that proves
// the directory is up. (Cross-peer registration over the Manager's mTLS API is
// out of scope.)
type PeerReconciler struct {
	Client client.Client
}

func (r *PeerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&openfscv1.Peer{}).
		Named("peer").
		Complete(r); err != nil {
		return fmt.Errorf("setup peer controller: %w", err)
	}
	return nil
}

func (r *PeerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var peer openfscv1.Peer
	if err := r.Client.Get(ctx, req.NamespacedName, &peer); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get Peer: %w", err)
	}

	// directoryDeploymentsReady only observes the local Manager/Controller
	// Deployments, which describe this cluster's own directory — not a remote
	// peer's health. Reporting that signal on a user-created remote peer would
	// mislabel it Active whenever the local directory is up. Cross-peer health
	// tracking is out of scope, so hold remote peers at Pending.
	if !peer.Spec.Directory {
		setPeerStatus(&peer, openfscv1.PhasePending, "remote peer; status tracking not yet supported")
		if err := r.Client.Status().Update(ctx, &peer); err != nil {
			return ctrl.Result{}, fmt.Errorf("update Peer status: %w", err)
		}
		return ctrl.Result{RequeueAfter: resyncInterval}, nil
	}

	// The directory peer's namespace and Controller URL come from the Directory
	// resource that deployed it.
	dir, err := getDirectory(ctx, r.Client)
	if err != nil {
		setPeerStatus(&peer, openfscv1.PhaseError, err.Error())
		_ = r.Client.Status().Update(ctx, &peer)
		return ctrl.Result{}, err
	}
	if dir == nil {
		setPeerStatus(&peer, openfscv1.PhasePending, "no Directory resource found")
		if err := r.Client.Status().Update(ctx, &peer); err != nil {
			return ctrl.Result{}, fmt.Errorf("update Peer status: %w", err)
		}
		return ctrl.Result{RequeueAfter: pendingRetryInterval}, nil
	}

	ready, detail, err := directoryDeploymentsReady(ctx, r.Client, dir.Spec.Namespace)
	if err != nil {
		setPeerStatus(&peer, openfscv1.PhaseError, err.Error())
		_ = r.Client.Status().Update(ctx, &peer)
		return ctrl.Result{}, err
	}

	if ready {
		setPeerStatus(&peer, openfscv1.PhaseActive, "OpenFSC Manager and Controller are running")
	} else {
		setPeerStatus(&peer, openfscv1.PhasePending, detail)
	}
	// Surface the host-reachable Controller UI URL so a console can link to it.
	// Only the directory peer reaches this point (remote peers returned above).
	peer.Status.ControllerURL = dir.Spec.ControllerURL
	if err := r.Client.Status().Update(ctx, &peer); err != nil {
		return ctrl.Result{}, fmt.Errorf("update Peer status: %w", err)
	}

	if ready {
		return ctrl.Result{RequeueAfter: resyncInterval}, nil
	}
	return ctrl.Result{RequeueAfter: pendingRetryInterval}, nil
}

// setPeerStatus updates the Peer's phase, message, observedGeneration,
// lastSyncedTime and the Ready condition.
func setPeerStatus(peer *openfscv1.Peer, phase openfscv1.Phase, message string) {
	now := metav1.Now()
	peer.Status.Phase = phase
	peer.Status.Message = message
	peer.Status.ObservedGeneration = peer.Generation
	peer.Status.LastSyncedTime = &now

	condStatus := metav1.ConditionFalse
	if phase == openfscv1.PhaseActive {
		condStatus = metav1.ConditionTrue
	}
	meta.SetStatusCondition(&peer.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             condStatus,
		ObservedGeneration: peer.Generation,
		Reason:             string(phase),
		Message:            message,
	})
}
