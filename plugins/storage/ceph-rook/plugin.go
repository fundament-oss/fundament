package main

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
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

// buildScheme registers core Kubernetes types, CRDs, and this plugin's API.
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

// cacheOptions confines the manager's ConfigMap informer to Rook's per-node
// discovery ConfigMaps. The watch predicate only filters events; without this the
// cache holds every ConfigMap in the cluster.
//
// Anything the reconcilers read through the cached client must match these
// selectors. DiskInventoryReconciler only touches discovery ConfigMaps, and the
// install path uses its own uncached client.
func cacheOptions(cfg Config) cache.Options {
	return cache.Options{
		ByObject: map[client.Object]cache.ByObject{
			&corev1.ConfigMap{}: {
				Namespaces: map[string]cache.Config{
					cfg.RookNamespace: {
						LabelSelector: labels.SelectorFromSet(labels.Set{"app": discoverAppLabel}),
					},
				},
			},
		},
	}
}

func (p *Plugin) Start(ctx context.Context, host pluginruntime.Host) error {
	scheme, err := buildScheme()
	if err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("build scheme: %w", pluginerrors.NewPermanent(err))
	}

	kubeCfg, err := ctrl.GetConfig()
	if err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("get kubeconfig: %w", pluginerrors.NewPermanent(err))
	}

	// Uncached client for the install phase, before the manager starts.
	kube, err := client.New(kubeCfg, client.Options{Scheme: scheme})
	if err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("create kubernetes client: %w", pluginerrors.NewPermanent(err))
	}

	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseInstalling, Message: "installing rook-ceph operator"})
	if err := p.install(ctx, kube); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("install: %w", pluginerrors.NewTransient(err))
	}

	// Reconcilers register below, before the blocking mgr.Start.
	mgr, err := crhelper.SetupManager(scheme, &ctrl.Options{
		// The plugin host owns the lifecycle; no listeners of our own.
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
		Cache:                  cacheOptions(p.cfg),
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
		RookNamespace:    p.cfg.RookNamespace,
	}).SetupWithManager(mgr); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("setup storagepool reconciler: %w", pluginerrors.NewPermanent(err))
	}

	host.ReportReady()
	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: "rook-ceph storage plugin running"})

	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("manager stopped: %w", pluginerrors.NewTransient(err))
	}
	return nil
}

func (p *Plugin) Shutdown(_ context.Context) error { return nil }
