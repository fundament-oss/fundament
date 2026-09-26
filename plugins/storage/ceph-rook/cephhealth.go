package main

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

// cephHealthErr is Ceph's error-level health string; HEALTH_WARN is not it (a
// single-node cluster warns about redundancy and is still usable).
const cephHealthErr = "HEALTH_ERR"

// CephHealthReconciler mirrors the singleton CephCluster's health into the
// plugin's own status, so an operator sees "storage cluster unhealthy: <reason>"
// at the plugin level instead of a green plugin sitting over a broken cluster.
// It only reads the CephCluster; DiskPoolReconciler owns writing it.
type CephHealthReconciler struct {
	Client           client.Client
	ClusterNamespace string
	Host             pluginruntime.Host
}

func (r *CephHealthReconciler) SetupWithManager(mgr manager.Manager) error {
	// No predicate: status changes are exactly what this watches, and a
	// generation filter would drop them.
	if err := ctrl.NewControllerManagedBy(mgr).
		For(rookStub("CephCluster")).
		Complete(r); err != nil {
		return fmt.Errorf("register CephCluster health controller: %w", err)
	}
	return nil
}

func (r *CephHealthReconciler) Reconcile(ctx context.Context, _ ctrl.Request) (ctrl.Result, error) {
	cc := rookStub("CephCluster")
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: r.ClusterNamespace, Name: cephClusterName}, cc)
	if apierrors.IsNotFound(err) {
		r.Host.ReportStatus(pluginruntime.PluginStatus{
			Phase:   pluginruntime.PhaseDegraded,
			Message: fmt.Sprintf("CephCluster %s/%s not found", r.ClusterNamespace, cephClusterName),
		})
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("get CephCluster: %w", err)
	}
	r.Host.ReportStatus(cephClusterPluginStatus(cc))
	return ctrl.Result{}, nil
}

// cephClusterPluginStatus maps a CephCluster's status into a plugin status. A
// cluster that is not Ready, or that reports HEALTH_ERR, degrades the plugin
// and names the reason; Ready plus HEALTH_OK/HEALTH_WARN is Running (a
// single-node cluster is permanently HEALTH_WARN about redundancy yet works).
func cephClusterPluginStatus(cc *unstructured.Unstructured) pluginruntime.PluginStatus {
	phase, _, _ := unstructured.NestedString(cc.Object, "status", "phase")
	health, _, _ := unstructured.NestedString(cc.Object, "status", "ceph", "health")

	if phase != v1alpha1.PhaseReady {
		msg, _, _ := unstructured.NestedString(cc.Object, "status", "message")
		return pluginruntime.PluginStatus{
			Phase:   pluginruntime.PhaseDegraded,
			Message: cephDegradedMessage("storage cluster "+cephClusterPhaseText(phase), msg),
		}
	}
	if health == cephHealthErr {
		return pluginruntime.PluginStatus{
			Phase:   pluginruntime.PhaseDegraded,
			Message: cephDegradedMessage("storage cluster HEALTH_ERR", cephHealthSummary(cc)),
		}
	}
	return pluginruntime.PluginStatus{
		Phase:   pluginruntime.PhaseRunning,
		Message: "rook-ceph storage plugin running",
	}
}

// cephClusterPhaseText renders an empty phase (no status yet) as something
// legible rather than a blank.
func cephClusterPhaseText(phase string) string {
	if phase == "" {
		return "has no status yet"
	}
	return phase
}

// cephDegradedMessage joins a headline with an optional detail.
func cephDegradedMessage(headline, detail string) string {
	if detail == "" {
		return headline
	}
	return headline + ": " + detail
}

// cephHealthSummary pulls the first health-check message out of
// status.ceph.details, so HEALTH_ERR names a cause rather than just the code.
func cephHealthSummary(cc *unstructured.Unstructured) string {
	details, found, err := unstructured.NestedMap(cc.Object, "status", "ceph", "details")
	if err != nil || !found {
		return ""
	}
	for _, v := range details {
		entry, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if msg, ok := entry["message"].(string); ok && msg != "" {
			return msg
		}
	}
	return ""
}
