package main

import (
	"context"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
	pluginerrors "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/errors"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/crd"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/helm"
)

// TODO: point these at the chart this plugin installs.
const (
	releaseName  = "demo"
	chartName    = "demo"
	repoURL      = "https://charts.example.com"
	chartVersion = "v0.1.0"
	namespace    = "demo"
)

// expectedCRDs are verified after install and on every reconcile: a chart that
// installed but whose CRDs are missing is a degraded plugin, not a healthy one.
var expectedCRDs = []string{
	"widgets.example.com",
}

// DemoPlugin implements the demo Fundament plugin.
type DemoPlugin struct {
	helmClient *helm.Client
	k8sClient  client.Client
}

// NewDemoPlugin creates a new DemoPlugin.
func NewDemoPlugin() *DemoPlugin {
	return &DemoPlugin{
		helmClient: helm.NewClient(namespace),
	}
}

// Start installs the chart if it is not already installed, then blocks until ctx
// is cancelled.
//
// Start must be idempotent: the container is restarted on upgrades, node
// evictions and crashes, so this runs again from scratch every time. That is why
// it checks IsInstalled before installing.
func (p *DemoPlugin) Start(ctx context.Context, host pluginruntime.Host) error {
	installed, err := p.helmClient.IsInstalled(ctx, releaseName)
	if err != nil {
		return fmt.Errorf("check helm status: %w", pluginerrors.NewTransient(err))
	}

	if !installed {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseInstalling, Message: "installing demo"})
		if err := p.Install(ctx, host); err != nil {
			host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
			return fmt.Errorf("install demo: %w", pluginerrors.NewTransient(err))
		}
	}

	scheme := runtime.NewScheme()
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("add apiextensions to scheme: %w", pluginerrors.NewPermanent(err))
	}

	k8sClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("create kubernetes client: %w", pluginerrors.NewPermanent(err))
	}
	p.k8sClient = k8sClient

	if err := crd.VerifyAll(ctx, p.k8sClient, expectedCRDs); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("verify CRDs: %w", pluginerrors.NewTransient(err))
	}

	host.ReportReady()
	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: "demo is running"})

	<-ctx.Done()
	return nil
}

// Shutdown performs graceful cleanup. It must not uninstall the chart: shutdown
// happens on every restart, not only on uninstall.
func (p *DemoPlugin) Shutdown(_ context.Context) error {
	return nil
}

// Install installs the chart.
func (p *DemoPlugin) Install(ctx context.Context, _ pluginruntime.Host) error {
	if err := p.helmClient.InstallFromRepo(ctx, releaseName, chartName, repoURL, chartVersion, map[string]string{
		// TODO: chart values.
	}); err != nil {
		return fmt.Errorf("install from repo: %w", err)
	}
	return nil
}

// Uninstall removes the chart. It runs when the PluginInstallation is deleted.
func (p *DemoPlugin) Uninstall(ctx context.Context, _ pluginruntime.Host) error {
	if err := p.helmClient.Uninstall(ctx, releaseName); err != nil {
		return fmt.Errorf("uninstall: %w", err)
	}
	return nil
}

// Upgrade re-runs the install, which Helm treats as an upgrade.
func (p *DemoPlugin) Upgrade(ctx context.Context, host pluginruntime.Host) error {
	return p.Install(ctx, host)
}

// Reconcile re-checks the plugin's health every FUNP_RECONCILE_INTERVAL
// (default 5m) and reports the result.
func (p *DemoPlugin) Reconcile(ctx context.Context, host pluginruntime.Host) error {
	if p.k8sClient == nil {
		return nil
	}

	if err := crd.VerifyAll(ctx, p.k8sClient, expectedCRDs); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("reconcile: CRDs missing: %w", pluginerrors.NewTransient(err))
	}

	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: "demo is running"})
	return nil
}
