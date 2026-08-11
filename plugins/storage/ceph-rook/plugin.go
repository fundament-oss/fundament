package main

import (
	"context"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
	pluginerrors "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/errors"
	crhelper "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/controllerruntime"
	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

// Plugin is the Ceph/Rook storage plugin.
type Plugin struct {
	cfg Config
}

// NewPlugin loads config and returns the plugin.
func NewPlugin() (*Plugin, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	return &Plugin{cfg: cfg}, nil
}

// buildScheme builds the runtime Scheme for this plugin. It includes the
// core Kubernetes types, the API extensions group (CRDs), and the plugin's
// own storage.fundament.io/v1alpha1 API types.
func buildScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		clientgoscheme.AddToScheme,
		apiextensionsv1.AddToScheme,
		v1alpha1.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			return nil, err
		}
	}
	return scheme, nil
}

func (p *Plugin) Start(ctx context.Context, host pluginruntime.Host) error {
	// Build the runtime scheme.
	scheme, err := buildScheme()
	if err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("build scheme: %w", pluginerrors.NewPermanent(err))
	}

	// Obtain the in-cluster kubeconfig.
	kubeCfg, err := ctrl.GetConfig()
	if err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("get kubeconfig: %w", pluginerrors.NewPermanent(err))
	}

	// Create a plain client used during the install phase (before the manager
	// is started and its cache is warm).
	kube, err := client.New(kubeCfg, client.Options{Scheme: scheme})
	if err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("create kubernetes client: %w", pluginerrors.NewPermanent(err))
	}

	// Install rook, apply CRDs, and bootstrap the CephCluster singleton.
	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseInstalling, Message: "installing rook-ceph operator"})
	if err := p.install(ctx, kube); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("install: %w", pluginerrors.NewTransient(err))
	}

	// Create the controller-runtime manager. Reconcilers register against it
	// below, before the blocking mgr.Start call.
	mgr, err := crhelper.SetupManager(scheme, &ctrl.Options{
		// Disable the default metrics and health-probe listeners; the plugin
		// host manages the plugin lifecycle.
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
	})
	if err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("setup controller manager: %w", pluginerrors.NewPermanent(err))
	}
	if err := (&DiskInventoryReconciler{
		Client:        mgr.GetClient(),
		RookNamespace: p.cfg.RookNamespace,
		LoopDevices:   p.cfg.DevLoopDevices,
	}).SetupWithManager(mgr); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("setup disk inventory reconciler: %w", pluginerrors.NewPermanent(err))
	}

	if err := (&StoragePoolReconciler{
		Client:           mgr.GetClient(),
		ClusterNamespace: p.cfg.ClusterNamespace,
	}).SetupWithManager(mgr); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("setup storagepool reconciler: %w", pluginerrors.NewPermanent(err))
	}

	host.ReportReady()
	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: "rook-ceph storage plugin running"})

	// mgr.Start blocks until ctx is cancelled.
	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("manager stopped: %w", pluginerrors.NewTransient(err))
	}
	return nil
}

func (p *Plugin) Shutdown(_ context.Context) error { return nil }
