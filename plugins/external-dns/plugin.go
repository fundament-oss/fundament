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

const (
	releaseName  = "external-dns"
	chartName    = "external-dns"
	repoURL      = "https://kubernetes-sigs.github.io/external-dns/"
	chartVersion = "1.16.1"
	namespace    = "external-dns"
)

var externalDNSCRDs = []string{
	"dnsendpoints.externaldns.k8s.io",
}

// ExternalDNSPlugin implements the external-dns Fundament plugin.
type ExternalDNSPlugin struct {
	def        pluginruntime.PluginDefinition
	helmClient *helm.Client
	k8sClient  client.Client
}

// NewExternalDNSPlugin creates a new ExternalDNSPlugin with the given definition.
func NewExternalDNSPlugin(def *pluginruntime.PluginDefinition) *ExternalDNSPlugin {
	return &ExternalDNSPlugin{
		def:        *def,
		helmClient: helm.NewClient(namespace),
	}
}

func (p *ExternalDNSPlugin) Definition() pluginruntime.PluginDefinition {
	return p.def
}

func (p *ExternalDNSPlugin) Start(ctx context.Context, host pluginruntime.Host) error {
	installed, err := p.helmClient.IsInstalled(ctx, releaseName)
	if err != nil {
		return fmt.Errorf("check helm status: %w", pluginerrors.NewTransient(err))
	}

	if !installed {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseInstalling, Message: "installing external-dns"})
		if err := p.Install(ctx, host); err != nil {
			host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
			return fmt.Errorf("install external-dns: %w", pluginerrors.NewTransient(err))
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

	if err := crd.VerifyAll(ctx, p.k8sClient, externalDNSCRDs); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("verify CRDs: %w", pluginerrors.NewTransient(err))
	}

	host.ReportReady()
	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: "external-dns is running"})

	<-ctx.Done()
	return nil
}

func (p *ExternalDNSPlugin) Shutdown(_ context.Context) error {
	return nil
}

func (p *ExternalDNSPlugin) Install(ctx context.Context, _ pluginruntime.Host) error {
	if err := p.helmClient.InstallFromRepo(ctx, releaseName, chartName, repoURL, chartVersion, map[string]string{
		"sources": "{crd}",
	}); err != nil {
		return fmt.Errorf("install from repo: %w", err)
	}
	return nil
}

func (p *ExternalDNSPlugin) Uninstall(ctx context.Context, _ pluginruntime.Host) error {
	if err := p.helmClient.Uninstall(ctx, releaseName); err != nil {
		return fmt.Errorf("uninstall: %w", err)
	}
	return nil
}

func (p *ExternalDNSPlugin) Upgrade(ctx context.Context, host pluginruntime.Host) error {
	return p.Install(ctx, host)
}

func (p *ExternalDNSPlugin) Reconcile(ctx context.Context, host pluginruntime.Host) error {
	if p.k8sClient == nil {
		return nil
	}

	if err := crd.VerifyAll(ctx, p.k8sClient, externalDNSCRDs); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("reconcile: CRDs missing: %w", pluginerrors.NewTransient(err))
	}

	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: "external-dns is running"})
	return nil
}
