package controller

import (
	"context"
	"fmt"
	"time"

	"helm.sh/helm/v3/pkg/chart"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openfscv1 "github.com/fundament-oss/fundament/openfsc-operator/pkg/api/v1"
	"github.com/fundament-oss/fundament/openfsc-operator/pkg/helm"
)

// --- gateway reconcilers (Inway, Outway): provision then observe ----------
//
// The CR is the source of truth: the reconciler provisions the gateway —
// mint its mTLS certs (cert-manager), then install the vendored
// open-fsc-inway / open-fsc-outway chart (one Helm release per CR). The
// deployed gateway self-registers with the OpenFSC Controller; the reconciler
// then polls the Controller Administration API to confirm registration and
// flips the CR to Active. A finalizer tears the gateway (Helm release + certs)
// back down on delete. Status guards (compared by condition reason) keep the
// self-triggered status writes from looping.
//
// The gateway's namespace, group and peer identity come from the cluster's
// Directory resource. The namespace is pinned on the CR in an annotation when
// provisioning starts, so the finalizer can tear down even after the Directory
// itself is deleted.

// gatewayFinalizer guarantees the gateway's Helm release and certs are removed
// before the CR disappears.
const gatewayFinalizer = "openfsc.fundament.io/gateway"

// gatewayNamespaceAnnotation records the namespace a gateway was provisioned
// in (always set before its certs/release are created).
const gatewayNamespaceAnnotation = "openfsc.fundament.io/namespace"

// handleGatewayLifecycle covers the shared finalizer dance and Directory
// lookup of the Inway/Outway reconcilers. It returns (directory, namespace,
// result, done): when done is true the caller returns result immediately
// (deletion handled, finalizer added, or no Directory yet); otherwise the
// gateway should be provisioned into namespace.
func handleGatewayLifecycle(ctx context.Context, c, certs client.Client, obj client.Object, st *openfscv1.Status, gen int64) (*openfscv1.Directory, string, ctrl.Result, bool, error) {
	name := obj.GetName()

	if !obj.GetDeletionTimestamp().IsZero() {
		if controllerutil.ContainsFinalizer(obj, gatewayFinalizer) {
			// The annotation is set before anything is provisioned; when it is
			// absent there is nothing to tear down.
			ns := obj.GetAnnotations()[gatewayNamespaceAnnotation]
			if ns != "" {
				if err := teardownGateway(ctx, certs, ns, name); err != nil {
					return nil, "", ctrl.Result{}, true, err
				}
			}
			controllerutil.RemoveFinalizer(obj, gatewayFinalizer)
			if err := c.Update(ctx, obj); err != nil {
				return nil, "", ctrl.Result{}, true, fmt.Errorf("remove finalizer: %w", err)
			}
		}
		return nil, "", ctrl.Result{}, true, nil
	}
	if controllerutil.AddFinalizer(obj, gatewayFinalizer) {
		if err := c.Update(ctx, obj); err != nil {
			return nil, "", ctrl.Result{}, true, fmt.Errorf("add finalizer: %w", err)
		}
		return nil, "", ctrl.Result{Requeue: true}, true, nil
	}

	dir, err := getDirectory(ctx, c)
	if err != nil {
		res, e := reflectError(ctx, c, obj, st, gen, err.Error())
		return nil, "", res, true, e
	}
	if dir == nil {
		res, e := reportPending(ctx, c, obj, st, gen, "NoDirectory",
			"no Directory resource found; create one to deploy the OpenFSC directory", pendingRetryInterval)
		return nil, "", res, true, e
	}

	// Pin the namespace before provisioning anything into it, so teardown works
	// even when the Directory is deleted first.
	ns := dir.Spec.Namespace
	if obj.GetAnnotations()[gatewayNamespaceAnnotation] != ns {
		annotations := obj.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[gatewayNamespaceAnnotation] = ns
		obj.SetAnnotations(annotations)
		if err := c.Update(ctx, obj); err != nil {
			return nil, "", ctrl.Result{}, true, fmt.Errorf("pin gateway namespace: %w", err)
		}
		return nil, "", ctrl.Result{Requeue: true}, true, nil
	}

	return dir, ns, ctrl.Result{}, false, nil
}

// provisionGateway ensures the gateway's certs exist and its Helm release is
// installed. It returns done=true when provisioning is complete and the caller
// should proceed to observe registration; otherwise it returns the Result/err to
// return from Reconcile (certs still issuing, install error, etc.).
func provisionGateway(ctx context.Context, c, certs client.Client, obj client.Object, st *openfscv1.Status, gen int64, ns, peerID, name string, chrt *chart.Chart, values map[string]string) (ctrl.Result, bool, error) {
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

	helmClient := helm.NewClient(ns)
	installed, err := helmClient.IsInstalled(name)
	if err != nil {
		res, e := reflectError(ctx, c, obj, st, gen, err.Error())
		return res, false, e
	}
	if !installed {
		vals, err := helm.SetValues(values)
		if err == nil {
			err = helmClient.UpgradeInstall(ctx, name, chrt, vals)
		}
		if err != nil {
			res, e := reflectError(ctx, c, obj, st, gen, err.Error())
			return res, false, e
		}
	}
	return ctrl.Result{}, true, nil
}

// teardownGateway removes the gateway's Helm release and certificates.
func teardownGateway(ctx context.Context, certs client.Client, ns, name string) error {
	if err := helm.NewClient(ns).Uninstall(name); err != nil {
		return fmt.Errorf("uninstall gateway release %s: %w", name, err)
	}
	return deleteGatewayCerts(ctx, certs, ns, name)
}
