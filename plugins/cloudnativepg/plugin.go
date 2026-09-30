package main

import (
	"context"
	"fmt"
	"sync/atomic"

	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
	pluginerrors "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/errors"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/crd"
	"github.com/fundament-oss/fundament/plugins/internal/cnpg"
)

// healthyPhase is the status.phase CNPG reports for a Cluster that is up.
const healthyPhase = "Cluster in healthy state"

var clusterListGVK = schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "ClusterList"}

// Plugin installs the shared CloudNativePG operator and lets project members
// create single-instance PostgreSQL databases (CNPG Clusters) from the console.
// It runs no controller of its own: the operator reconciles the Clusters, so
// databases keep running while this pod is down.
type Plugin struct {
	scheme *runtime.Scheme
	// kube is set by Start and read by Reconcile, which runs concurrently.
	kube atomic.Pointer[client.Client]
}

func newPlugin() (*Plugin, error) {
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		clientgoscheme.AddToScheme,
		apiextensionsv1.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			return nil, fmt.Errorf("build scheme: %w", err)
		}
	}
	return &Plugin{scheme: scheme}, nil
}

func (p *Plugin) Start(ctx context.Context, host pluginruntime.Host) error {
	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseInstalling, Message: "installing the CloudNativePG operator"})
	if err := cnpg.Install(ctx); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("install operator: %w", pluginerrors.NewTransient(err))
	}

	kube, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: p.scheme})
	if err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("create kubernetes client: %w", pluginerrors.NewPermanent(err))
	}

	if err := crd.VerifyAll(ctx, kube, cnpg.CRDNames); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("verify CRDs: %w", pluginerrors.NewTransient(err))
	}

	if err := applyUserRoles(ctx, kube); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("apply user roles: %w", pluginerrors.NewTransient(err))
	}

	p.kube.Store(&kube)
	host.ReportReady()
	reportStatus(ctx, host, kube)

	<-ctx.Done()
	return nil
}

func (p *Plugin) Shutdown(_ context.Context) error {
	return nil
}

func (p *Plugin) Reconcile(ctx context.Context, host pluginruntime.Host) error {
	kube := p.kube.Load()
	if kube == nil {
		return nil
	}
	if err := crd.VerifyAll(ctx, *kube, cnpg.CRDNames); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("reconcile: CRDs missing: %w", pluginerrors.NewTransient(err))
	}
	reportStatus(ctx, host, *kube)
	return nil
}

// reportStatus summarizes the cluster's databases in the plugin status. The
// plugin is a capability provider: it stays Running with zero databases, and
// per-database health lives on the Cluster resources.
func reportStatus(ctx context.Context, host pluginruntime.Host, kube client.Client) {
	message, err := statusMessage(ctx, kube)
	if err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return
	}
	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: message})
}

func statusMessage(ctx context.Context, kube client.Client) (string, error) {
	clusters := &unstructured.UnstructuredList{}
	clusters.SetGroupVersionKind(clusterListGVK)
	if err := kube.List(ctx, clusters); err != nil {
		return "", fmt.Errorf("list databases: %w", err)
	}

	message := "CloudNativePG operator running; no databases yet"
	if n := len(clusters.Items); n > 0 {
		healthy := 0
		for i := range clusters.Items {
			phase, _, _ := unstructured.NestedString(clusters.Items[i].Object, "status", "phase")
			if phase == healthyPhase {
				healthy++
			}
		}
		noun := "databases"
		if n == 1 {
			noun = "database"
		}
		message = fmt.Sprintf("%d %s: %d healthy, %d provisioning", n, noun, healthy, n-healthy)
	}

	hasDefault, err := hasDefaultStorageClass(ctx, kube)
	if err != nil {
		return "", err
	}
	if !hasDefault {
		// Not Degraded: a database can still name a StorageClass explicitly.
		message += "; no default StorageClass, pick one when creating a database"
	}
	return message, nil
}

func hasDefaultStorageClass(ctx context.Context, kube client.Client) (bool, error) {
	var classes storagev1.StorageClassList
	if err := kube.List(ctx, &classes); err != nil {
		return false, fmt.Errorf("list StorageClasses: %w", err)
	}
	for i := range classes.Items {
		annotations := classes.Items[i].Annotations
		if annotations["storageclass.kubernetes.io/is-default-class"] == "true" ||
			annotations["storageclass.beta.kubernetes.io/is-default-class"] == "true" {
			return true, nil
		}
	}
	return false, nil
}
