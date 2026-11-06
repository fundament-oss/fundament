package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
	pluginerrors "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/errors"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/crd"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/helm"
)

const (
	releaseName  = "cert-manager"
	chartName    = "cert-manager"
	repoURL      = "https://charts.jetstack.io"
	chartVersion = "v1.21.3"
	namespace    = "cert-manager"
)

var certManagerCRDs = []string{
	"certificates.cert-manager.io",
	"certificaterequests.cert-manager.io",
	"issuers.cert-manager.io",
	"clusterissuers.cert-manager.io",
}

// gatewayAPICRDs are the Gateway API resources cert-manager watches: the
// annotated Gateways it issues certificates for, and the HTTPRoutes of its
// HTTP-01 solver.
var gatewayAPICRDs = []string{
	"gateways.gateway.networking.k8s.io",
	"httproutes.gateway.networking.k8s.io",
}

// chartValues returns the chart values. The cert-manager controller exits at
// start when Gateway API support is on and the Gateway API CRDs are absent, so
// the support is on only while they are installed.
func chartValues(gatewayAPI bool) map[string]string {
	values := map[string]string{"crds.enabled": "true"}
	if gatewayAPI {
		values["config.gatewayAPI.enabled"] = "true"
	}
	return values
}

func runningMessage(gatewayAPI bool) string {
	if gatewayAPI {
		return "cert-manager is running"
	}
	return "cert-manager is running; Gateway API support is off"
}

// CertManagerPlugin implements the cert-manager Fundament plugin.
type CertManagerPlugin struct {
	helmClient *helm.Client
	k8sClient  client.Client
	// running is set once Start has installed and verified cert-manager.
	// Reconcile does nothing before.
	running atomic.Bool
	// helmMu keeps an uninstall and a Gateway API reinstall from overlapping.
	helmMu sync.Mutex
}

// NewCertManagerPlugin creates a new CertManagerPlugin.
func NewCertManagerPlugin() *CertManagerPlugin {
	return &CertManagerPlugin{
		helmClient: helm.NewClient(namespace),
	}
}

func (p *CertManagerPlugin) Start(ctx context.Context, host pluginruntime.Host) error {
	scheme := runtime.NewScheme()
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("add apiextensions to scheme: %w", pluginerrors.NewPermanent(err))
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("add core to scheme: %w", pluginerrors.NewPermanent(err))
	}

	k8sClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("create kubernetes client: %w", pluginerrors.NewPermanent(err))
	}
	p.k8sClient = k8sClient

	installed, err := p.helmClient.IsInstalled(ctx, releaseName)
	if err != nil {
		return fmt.Errorf("check helm status: %w", pluginerrors.NewTransient(err))
	}

	if !installed {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseInstalling, Message: "installing cert-manager"})
		if err := p.Install(ctx, host); err != nil {
			host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
			return fmt.Errorf("install cert-manager: %w", pluginerrors.NewTransient(err))
		}
	}

	if err := crd.VerifyAll(ctx, p.k8sClient, certManagerCRDs); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("verify CRDs: %w", pluginerrors.NewTransient(err))
	}

	gatewayAPI, err := p.gatewayAPIEnabled(ctx)
	if err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("read controller config: %w", pluginerrors.NewTransient(err))
	}

	p.running.Store(true)
	host.ReportReady()
	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: runningMessage(gatewayAPI)})

	<-ctx.Done()
	return nil
}

func (p *CertManagerPlugin) Shutdown(_ context.Context) error {
	return nil
}

func (p *CertManagerPlugin) Install(ctx context.Context, _ pluginruntime.Host) error {
	gatewayAPI, err := p.gatewayAPIInstalled(ctx)
	if err != nil {
		return err
	}
	if err := p.helmClient.InstallFromRepo(ctx, releaseName, chartName, repoURL, chartVersion, chartValues(gatewayAPI)); err != nil {
		return fmt.Errorf("install from repo: %w", err)
	}
	return nil
}

func (p *CertManagerPlugin) Uninstall(ctx context.Context, _ pluginruntime.Host) error {
	p.helmMu.Lock()
	defer p.helmMu.Unlock()
	if err := p.helmClient.Uninstall(ctx, releaseName); err != nil {
		return fmt.Errorf("uninstall: %w", err)
	}
	return nil
}

func (p *CertManagerPlugin) Upgrade(ctx context.Context, host pluginruntime.Host) error {
	return p.Install(ctx, host)
}

func (p *CertManagerPlugin) Reconcile(ctx context.Context, host pluginruntime.Host) error {
	if !p.running.Load() {
		return nil
	}

	if err := crd.VerifyAll(ctx, p.k8sClient, certManagerCRDs); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("reconcile: CRDs missing: %w", pluginerrors.NewTransient(err))
	}

	gatewayAPI, err := p.syncGatewayAPI(ctx, host)
	if err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("reconcile: gateway API: %w", pluginerrors.NewTransient(err))
	}

	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: runningMessage(gatewayAPI)})
	return nil
}

// syncGatewayAPI reinstalls the release when the Gateway API CRDs were
// installed or removed after cert-manager was configured, and returns whether
// Gateway API support is on. It changes only a deployed release of the
// plugin's chart version: cert-manager upgrades one minor version at a time.
func (p *CertManagerPlugin) syncGatewayAPI(ctx context.Context, host pluginruntime.Host) (bool, error) {
	p.helmMu.Lock()
	defer p.helmMu.Unlock()

	enabled, err := p.gatewayAPIEnabled(ctx)
	if err != nil {
		return false, err
	}
	installed, err := p.gatewayAPIInstalled(ctx)
	if err != nil || installed == enabled {
		return enabled, err
	}

	status, err := p.helmClient.Status(ctx, releaseName)
	if err != nil {
		return enabled, fmt.Errorf("check helm status: %w", err)
	}
	if status == nil {
		return enabled, nil
	}
	if status.Status != "deployed" || status.ChartVersion != chartVersion {
		if enabled {
			return enabled, fmt.Errorf("the Gateway API CRDs are missing while cert-manager has Gateway API support on, so its controller cannot start; release %s is %s at %s, which the plugin leaves alone",
				releaseName, status.Status, status.ChartVersion)
		}
		return enabled, nil
	}

	host.Logger().Info("Gateway API CRDs changed, reinstalling cert-manager", "gatewayAPI", installed)
	if err := p.Install(ctx, host); err != nil {
		return enabled, err
	}
	return installed, nil
}

// gatewayAPIInstalled reports whether the Gateway API CRDs cert-manager needs
// are installed.
func (p *CertManagerPlugin) gatewayAPIInstalled(ctx context.Context) (bool, error) {
	for _, name := range gatewayAPICRDs {
		ok, err := crd.Exists(ctx, p.k8sClient, name)
		if err != nil || !ok {
			return false, err //nolint:wrapcheck // crd.Exists names the CRD
		}
	}
	return true, nil
}

// gatewayAPIEnabled reports whether the running controller is configured with
// Gateway API support, from the ConfigMap the chart renders its config into.
func (p *CertManagerPlugin) gatewayAPIEnabled(ctx context.Context) (bool, error) {
	var cm corev1.ConfigMap
	err := p.k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: releaseName}, &cm)
	if client.IgnoreNotFound(err) != nil {
		return false, fmt.Errorf("get controller config: %w", err)
	}
	return parseGatewayAPIEnabled(cm.Data["config.yaml"])
}

func parseGatewayAPIEnabled(config string) (bool, error) {
	var cfg struct {
		GatewayAPI struct {
			Enabled bool `json:"enabled"`
		} `json:"gatewayAPI"`
	}
	if err := yaml.Unmarshal([]byte(config), &cfg); err != nil {
		return false, fmt.Errorf("parse controller config: %w", err)
	}
	return cfg.GatewayAPI.Enabled, nil
}
