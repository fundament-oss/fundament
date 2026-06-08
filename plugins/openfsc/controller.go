package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openfscv1 "github.com/fundament-oss/fundament/plugins/openfsc/api/v1"
	"github.com/fundament-oss/fundament/plugins/openfsc/controllerclient"
)

// OpenFSC `shared` umbrella Deployment names (release "shared" + chart name).
const (
	managerDeployment    = "shared-open-fsc-manager"
	controllerDeployment = "shared-open-fsc-controller"
)

const (
	// resyncInterval is the steady-state requeue once a Peer is Active.
	resyncInterval = 5 * time.Minute
	// pendingRetryInterval requeues a Pending Peer while the directory comes up.
	pendingRetryInterval = 15 * time.Second
)

// PeerReconciler reconciles Peer resources. For the directory peer this plugin
// installs, "Active" means the OpenFSC Manager and Controller Deployments are
// Available — a dependency-free readiness signal that proves the directory is
// up. (Cross-peer registration over the Manager's mTLS API is out of scope.)
type PeerReconciler struct {
	client        client.Client
	namespace     string
	controllerURL string
}

func (r *PeerReconciler) setupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openfscv1.Peer{}).
		Complete(r)
}

func (r *PeerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var peer openfscv1.Peer
	if err := r.client.Get(ctx, req.NamespacedName, &peer); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// directoryReady only observes the local Manager/Controller Deployments, which
	// describe this cluster's own directory — not a remote peer's health. Reporting
	// that signal on a user-created remote peer would mislabel it Active whenever
	// the local directory is up. Cross-peer health tracking is out of scope, so
	// hold remote peers at Pending.
	if !peer.Spec.Directory {
		setPeerStatus(&peer, openfscv1.PhasePending, "remote peer; status tracking not yet supported")
		if err := r.client.Status().Update(ctx, &peer); err != nil {
			return ctrl.Result{}, fmt.Errorf("update Peer status: %w", err)
		}
		return ctrl.Result{RequeueAfter: resyncInterval}, nil
	}

	ready, detail, err := r.directoryReady(ctx)
	if err != nil {
		setPeerStatus(&peer, openfscv1.PhaseError, err.Error())
		_ = r.client.Status().Update(ctx, &peer)
		return ctrl.Result{}, err
	}

	if ready {
		setPeerStatus(&peer, openfscv1.PhaseActive, "OpenFSC Manager and Controller are running")
	} else {
		setPeerStatus(&peer, openfscv1.PhasePending, detail)
	}
	// Surface the host-reachable Controller UI URL so the console can link to it.
	// Only the directory peer reaches this point (remote peers returned above).
	peer.Status.ControllerURL = r.controllerURL
	if err := r.client.Status().Update(ctx, &peer); err != nil {
		return ctrl.Result{}, fmt.Errorf("update Peer status: %w", err)
	}

	if ready {
		return ctrl.Result{RequeueAfter: resyncInterval}, nil
	}
	return ctrl.Result{RequeueAfter: pendingRetryInterval}, nil
}

// directoryReady reports whether both the Manager and Controller Deployments are
// Available, with a human-readable detail when they are not.
func (r *PeerReconciler) directoryReady(ctx context.Context) (bool, string, error) {
	for _, name := range []string{managerDeployment, controllerDeployment} {
		var deploy appsv1.Deployment
		err := r.client.Get(ctx, types.NamespacedName{Namespace: r.namespace, Name: name}, &deploy)
		if apierrors.IsNotFound(err) {
			return false, fmt.Sprintf("waiting for Deployment %s/%s", r.namespace, name), nil
		}
		if err != nil {
			return false, "", fmt.Errorf("get Deployment %s/%s: %w", r.namespace, name, err)
		}
		if !deploymentAvailable(&deploy) {
			return false, fmt.Sprintf("Deployment %s/%s not yet Available", r.namespace, name), nil
		}
	}
	return true, "", nil
}

func deploymentAvailable(deploy *appsv1.Deployment) bool {
	for _, cond := range deploy.Status.Conditions {
		if cond.Type == appsv1.DeploymentAvailable {
			return cond.Status == "True"
		}
	}
	return false
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

// --- gateway reconcilers (Inway, Outway): provision then observe ----------
//
// The CR is the source of truth: the reconciler provisions the gateway —
// mint its mTLS certs (cert-manager), then helm-install the vendored
// open-fsc-inway / open-fsc-outway chart (one release per CR). The deployed
// gateway self-registers with the OpenFSC Controller; the reconciler then polls
// the Controller Administration API to confirm registration and flips the CR to
// Active. A finalizer tears the gateway (helm release + certs) back down on
// delete. Status guards (compared by condition reason) keep the self-triggered
// status writes from looping.

// gatewayRequeue is the steady-state poll interval for observing registrations.
const gatewayRequeue = time.Minute

// gatewayFinalizer guarantees the gateway's helm release and certs are removed
// before the CR disappears.
const gatewayFinalizer = "openfsc.fundament.io/gateway"

// Controller Administration API subsets, each satisfied by *controllerclient.Client.
type (
	inwayAdminAPI interface {
		ListInways(ctx context.Context) ([]controllerclient.Inway, error)
	}
	outwayAdminAPI interface {
		ListOutways(ctx context.Context) ([]controllerclient.Outway, error)
	}
)

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
		return ctrl.Result{}, err
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
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: gatewayRequeue}, nil
}

// reportNotConfigured records that the operator has no Controller Administration
// API wiring, so the gateway's registration cannot be observed yet.
func reportNotConfigured(ctx context.Context, c client.Client, obj client.Object, st *openfscv1.Status, gen int64) (ctrl.Result, error) {
	const reason = "NotConfigured"
	cond := meta.FindStatusCondition(st.Conditions, "Synced")
	if st.ObservedGeneration == gen && cond != nil && cond.Reason == reason {
		return ctrl.Result{RequeueAfter: gatewayRequeue}, nil
	}
	setPending(st, gen, reason, "OpenFSC Controller Administration API not configured")
	if err := c.Status().Update(ctx, obj); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: gatewayRequeue}, nil
}

// reportPending records a not-yet-ready status with a custom reason, skipping the
// write when the same reason is already recorded for the current generation.
func reportPending(ctx context.Context, c client.Client, obj client.Object, st *openfscv1.Status, gen int64, reason, msg string, requeue time.Duration) (ctrl.Result, error) {
	cond := meta.FindStatusCondition(st.Conditions, "Synced")
	if !(st.ObservedGeneration == gen && cond != nil && cond.Reason == reason) {
		setPending(st, gen, reason, msg)
		if err := c.Status().Update(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{RequeueAfter: requeue}, nil
}

// InwayReconciler provisions the declared Inway (certs + open-fsc-inway chart)
// and reflects its registration with the Controller.
type InwayReconciler struct {
	client    client.Client // cached: Inway CR + status
	certs     client.Client // direct: cert-manager Certificates
	api       inwayAdminAPI // observe registration (Administration API)
	chartPath string
	namespace string
	groupID   string
	peerID    string
}

func (r *InwayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	obj := &openfscv1.Inway{}
	if err := r.client.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	name := obj.GetName()
	gen := obj.GetGeneration()
	st := &obj.Status

	if !obj.GetDeletionTimestamp().IsZero() {
		if controllerutil.ContainsFinalizer(obj, gatewayFinalizer) {
			if err := r.teardown(ctx, name); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(obj, gatewayFinalizer)
			if err := r.client.Update(ctx, obj); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}
	if controllerutil.AddFinalizer(obj, gatewayFinalizer) {
		if err := r.client.Update(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if res, done, err := provisionGateway(ctx, r.client, r.certs, obj, st, gen, r.namespace, r.peerID,
		name, r.chartPath, inwayValues(r.namespace, name, obj.Spec.InwayName, r.groupID)); !done {
		return res, err
	}

	if r.api == nil {
		return reportNotConfigured(ctx, r.client, obj, st, gen)
	}
	inways, err := r.api.ListInways(ctx)
	if errors.Is(err, errAdminNotConfigured) {
		return reportNotConfigured(ctx, r.client, obj, st, gen)
	}
	if err != nil {
		return reflectError(ctx, r.client, obj, st, gen, err.Error())
	}
	var addr string
	for _, iw := range inways {
		if iw.Name == obj.Spec.InwayName {
			addr = iw.Address
		}
	}
	return reflectRegistration(ctx, r.client, obj, st, gen, addr != "",
		fmt.Sprintf("Inway registered at %s", addr),
		"inway deployed; waiting for it to register with the Controller")
}

func (r *InwayReconciler) teardown(ctx context.Context, name string) error {
	if err := helmUninstall(ctx, r.namespace, name); err != nil {
		return err
	}
	return deleteGatewayCerts(ctx, r.certs, r.namespace, name)
}

func (r *InwayReconciler) setupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(&openfscv1.Inway{}).Named("inway").Complete(r)
}

// OutwayReconciler provisions the declared Outway (certs + open-fsc-outway chart)
// and reflects its registration with the Controller.
type OutwayReconciler struct {
	client    client.Client
	certs     client.Client
	api       outwayAdminAPI
	chartPath string
	namespace string
	groupID   string
	peerID    string
}

func (r *OutwayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	obj := &openfscv1.Outway{}
	if err := r.client.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	name := obj.GetName()
	gen := obj.GetGeneration()
	st := &obj.Status

	if !obj.GetDeletionTimestamp().IsZero() {
		if controllerutil.ContainsFinalizer(obj, gatewayFinalizer) {
			if err := r.teardown(ctx, name); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(obj, gatewayFinalizer)
			if err := r.client.Update(ctx, obj); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}
	if controllerutil.AddFinalizer(obj, gatewayFinalizer) {
		if err := r.client.Update(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Surface the in-cluster consume endpoint (forward proxy on :80) so the
	// console can show how to reach services through this outway.
	url := fmt.Sprintf("http://%s.%s", name, r.namespace)
	if st.URL != url {
		st.URL = url
		if err := r.client.Status().Update(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
	}

	if res, done, err := provisionGateway(ctx, r.client, r.certs, obj, st, gen, r.namespace, r.peerID,
		name, r.chartPath, outwayValues(r.namespace, name, obj.Spec.OutwayName, r.groupID)); !done {
		return res, err
	}

	if r.api == nil {
		return reportNotConfigured(ctx, r.client, obj, st, gen)
	}
	outways, err := r.api.ListOutways(ctx)
	if errors.Is(err, errAdminNotConfigured) {
		return reportNotConfigured(ctx, r.client, obj, st, gen)
	}
	if err != nil {
		return reflectError(ctx, r.client, obj, st, gen, err.Error())
	}
	registered := false
	for _, ow := range outways {
		if ow.Name == obj.Spec.OutwayName {
			registered = true
		}
	}
	return reflectRegistration(ctx, r.client, obj, st, gen, registered,
		"Outway registered with the Controller",
		"outway deployed; waiting for it to register with the Controller")
}

func (r *OutwayReconciler) teardown(ctx context.Context, name string) error {
	if err := helmUninstall(ctx, r.namespace, name); err != nil {
		return err
	}
	return deleteGatewayCerts(ctx, r.certs, r.namespace, name)
}

func (r *OutwayReconciler) setupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(&openfscv1.Outway{}).Named("outway").Complete(r)
}

// provisionGateway ensures the gateway's certs exist and its helm release is
// installed. It returns done=true when provisioning is complete and the caller
// should proceed to observe registration; otherwise it returns the Result/err to
// return from Reconcile (certs still issuing, install error, etc.).
func provisionGateway(ctx context.Context, c, certs client.Client, obj client.Object, st *openfscv1.Status, gen int64, ns, peerID, name, chartPath string, values map[string]string) (ctrl.Result, bool, error) {
	ready, err := ensureGatewayCerts(ctx, certs, ns, name, peerID)
	if err != nil {
		res, e := reflectError(ctx, c, obj, st, gen, err.Error())
		return res, false, e
	}
	if !ready {
		res, e := reportPending(ctx, c, obj, st, gen, "AwaitingCertificates",
			"waiting for cert-manager to issue the gateway certificates", 10*time.Second)
		return res, false, e
	}

	installed, err := helmInstalled(ctx, ns, name)
	if err != nil {
		res, e := reflectError(ctx, c, obj, st, gen, err.Error())
		return res, false, e
	}
	if !installed {
		if err := helmUpgradeInstall(ctx, ns, name, chartPath, values); err != nil {
			res, e := reflectError(ctx, c, obj, st, gen, err.Error())
			return res, false, e
		}
	}
	return ctrl.Result{}, true, nil
}
