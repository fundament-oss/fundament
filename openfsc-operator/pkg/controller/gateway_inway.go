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

// InwayReconciler provisions the declared Inway (certs + open-fsc-inway chart)
// and reflects its registration with the Controller.
type InwayReconciler struct {
	Client client.Client // cached: Inway CR + status
	Certs  client.Client // direct: cert-manager Certificates
	Admin  *AdminClients // observe registration (Administration API)
	Chart  *chart.Chart  // embedded open-fsc-inway chart
}

func (r *InwayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := ctrl.NewControllerManagedBy(mgr).For(&openfscv1.Inway{}).Named("inway").Complete(r); err != nil {
		return fmt.Errorf("setup inway controller: %w", err)
	}
	return nil
}

func (r *InwayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	obj := &openfscv1.Inway{}
	if err := r.Client.Get(ctx, req.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get Inway: %w", err)
	}
	name := obj.GetName()
	gen := obj.GetGeneration()
	st := &obj.Status

	dir, ns, res, done, err := handleGatewayLifecycle(ctx, r.Client, r.Certs, obj, st, gen)
	if done {
		return res, err
	}

	if res, done, err := provisionGateway(ctx, r.Client, r.Certs, obj, st, gen, ns, dir.Spec.PeerID,
		name, r.Chart, inwayValues(ns, name, obj.Spec.InwayName, dir.Spec.GroupID)); !done {
		return res, err
	}

	api, err := r.Admin.forNamespace(ctx, ns)
	if errors.Is(err, errAdminNotConfigured) {
		return reportNotConfigured(ctx, r.Client, obj, st, gen)
	}
	if err != nil {
		return reflectError(ctx, r.Client, obj, st, gen, err.Error())
	}
	inways, err := api.ListInways(ctx)
	if err != nil {
		return reflectError(ctx, r.Client, obj, st, gen, err.Error())
	}
	var addr string
	for _, iw := range inways {
		if iw.Name == obj.Spec.InwayName {
			addr = iw.Address
		}
	}
	return reflectRegistration(ctx, r.Client, obj, st, gen, addr != "",
		fmt.Sprintf("Inway registered at %s", addr),
		"inway deployed; waiting for it to register with the Controller")
}
