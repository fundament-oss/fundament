package controller

import (
	"context"
	"errors"
	"fmt"

	"helm.sh/helm/v3/pkg/chart"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openfscv1 "github.com/fundament-oss/fundament/openfsc-operator/pkg/api/v1"
)

// OutwayReconciler provisions the declared Outway (certs + open-fsc-outway
// chart) and reflects its registration with the Controller.
type OutwayReconciler struct {
	Client client.Client // cached: Outway CR + status
	Certs  client.Client // direct: cert-manager Certificates
	Admin  *AdminClients // observe registration (Administration API)
	Chart  *chart.Chart  // embedded open-fsc-outway chart
}

func (r *OutwayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := ctrl.NewControllerManagedBy(mgr).For(&openfscv1.Outway{}).Named("outway").Complete(r); err != nil {
		return fmt.Errorf("setup outway controller: %w", err)
	}
	return nil
}

func (r *OutwayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	obj := &openfscv1.Outway{}
	if err := r.Client.Get(ctx, req.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get Outway: %w", err)
	}
	name := obj.GetName()
	gen := obj.GetGeneration()
	st := &obj.Status

	dir, ns, res, done, err := handleGatewayLifecycle(ctx, r.Client, r.Certs, obj, st, gen)
	if done {
		return res, err
	}

	// Surface the in-cluster consume endpoint (forward proxy on :80) so a
	// console can show how to reach services through this outway.
	url := fmt.Sprintf("http://%s.%s", name, ns)
	if st.URL != url {
		st.URL = url
		if err := r.Client.Status().Update(ctx, obj); err != nil {
			return ctrl.Result{}, fmt.Errorf("update status URL: %w", err)
		}
	}

	if res, done, err := provisionGateway(ctx, r.Client, r.Certs, obj, st, gen, ns, dir.Spec.PeerID,
		name, r.Chart, outwayValues(name, obj.Spec.OutwayName, dir.Spec.GroupID)); !done {
		return res, err
	}

	api, err := r.Admin.forNamespace(ctx, ns)
	if errors.Is(err, errAdminNotConfigured) {
		return reportNotConfigured(ctx, r.Client, obj, st, gen)
	}
	if err != nil {
		return reflectError(ctx, r.Client, obj, st, gen, err.Error())
	}
	outways, err := api.ListOutways(ctx)
	if err != nil {
		return reflectError(ctx, r.Client, obj, st, gen, err.Error())
	}
	registered := false
	for _, ow := range outways {
		if ow.Name == obj.Spec.OutwayName {
			registered = true
		}
	}
	return reflectRegistration(ctx, r.Client, obj, st, gen, registered,
		"Outway registered with the Controller",
		"outway deployed; waiting for it to register with the Controller")
}
