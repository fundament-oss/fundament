package main

import (
	"context"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pluginsdk "github.com/fundament-oss/fundament/plugin-sdk"
	pluginerrors "github.com/fundament-oss/fundament/plugin-sdk/errors"
	"github.com/fundament-oss/fundament/plugin-sdk/helpers/crd"
	"github.com/fundament-oss/fundament/plugin-sdk/helpers/helm"
)

const (
	releaseName = "cert-manager"
	chartName   = "cert-manager"
	repoURL     = "https://charts.jetstack.io"
	namespace   = "cert-manager"
)

var certManagerCRDs = []string{
	"certificates.cert-manager.io",
	"certificaterequests.cert-manager.io",
	"issuers.cert-manager.io",
	"clusterissuers.cert-manager.io",
}

// CertManagerPlugin implements the cert-manager Fundament plugin.
type CertManagerPlugin struct {
	def        pluginsdk.PluginDefinition
	helmClient *helm.Client
	k8sClient  client.Client
}

// NewCertManagerPlugin creates a new CertManagerPlugin with the given definition.
func NewCertManagerPlugin(def pluginsdk.PluginDefinition) *CertManagerPlugin {
	return &CertManagerPlugin{
		def:        def,
		helmClient: helm.NewClient(namespace),
	}
}

func (p *CertManagerPlugin) Definition() pluginsdk.PluginDefinition {
	return p.def
}

func (p *CertManagerPlugin) Start(ctx context.Context, host pluginsdk.Host) error {
	host.ReportStatus(pluginsdk.PluginStatus{Phase: pluginsdk.PhaseInstalling, Message: "installing cert-manager"})

	if err := p.Install(ctx, host); err != nil {
		host.ReportStatus(pluginsdk.PluginStatus{Phase: pluginsdk.PhaseFailed, Message: err.Error()})
		return pluginerrors.NewPermanent(fmt.Errorf("install cert-manager: %w", err))
	}

	scheme := runtime.NewScheme()
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		host.ReportStatus(pluginsdk.PluginStatus{Phase: pluginsdk.PhaseFailed, Message: err.Error()})
		return pluginerrors.NewPermanent(fmt.Errorf("add apiextensions to scheme: %w", err))
	}

	k8sClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		host.ReportStatus(pluginsdk.PluginStatus{Phase: pluginsdk.PhaseFailed, Message: err.Error()})
		return pluginerrors.NewPermanent(fmt.Errorf("create kubernetes client: %w", err))
	}
	p.k8sClient = k8sClient

	if err := crd.VerifyAll(ctx, p.k8sClient, certManagerCRDs); err != nil {
		host.ReportStatus(pluginsdk.PluginStatus{Phase: pluginsdk.PhaseDegraded, Message: err.Error()})
		return pluginerrors.NewTransient(fmt.Errorf("verify CRDs: %w", err))
	}

	host.ReportReady()
	host.ReportStatus(pluginsdk.PluginStatus{Phase: pluginsdk.PhaseRunning, Message: "cert-manager is running"})

	<-ctx.Done()
	return nil
}

func (p *CertManagerPlugin) Shutdown(_ context.Context) error {
	return nil
}

func (p *CertManagerPlugin) Install(ctx context.Context, _ pluginsdk.Host) error {
	return p.helmClient.InstallFromRepo(ctx, releaseName, chartName, repoURL, map[string]string{
		"crds.enabled": "true",
	})
}

func (p *CertManagerPlugin) Uninstall(ctx context.Context, _ pluginsdk.Host) error {
	return p.helmClient.Uninstall(ctx, releaseName)
}

func (p *CertManagerPlugin) Upgrade(ctx context.Context, host pluginsdk.Host) error {
	return p.Install(ctx, host)
}

func (p *CertManagerPlugin) Reconcile(ctx context.Context, host pluginsdk.Host) error {
	if p.k8sClient == nil {
		return nil
	}

	if err := crd.VerifyAll(ctx, p.k8sClient, certManagerCRDs); err != nil {
		host.ReportStatus(pluginsdk.PluginStatus{Phase: pluginsdk.PhaseDegraded, Message: err.Error()})
		return pluginerrors.NewTransient(fmt.Errorf("reconcile: CRDs missing: %w", err))
	}

	host.ReportStatus(pluginsdk.PluginStatus{Phase: pluginsdk.PhaseRunning, Message: "cert-manager is running"})
	return nil
}
