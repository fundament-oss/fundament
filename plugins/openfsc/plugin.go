package main

import (
	"context"
	"fmt"

	"github.com/caarlos0/env/v11"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openfscv1 "github.com/fundament-oss/fundament/openfsc-operator/api/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
	pluginerrors "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/errors"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/crd"
)

// OpenFSCPlugin installs the standalone openfsc-operator (with its
// prerequisites) and surfaces the FSCInstallation resources in the console.
// It never creates FSCInstallations — teams declare their own — and all
// reconciliation lives in the operator, so installations keep running when
// this plugin pod is down.
type OpenFSCPlugin struct {
	cfg    pluginConfig
	scheme *runtime.Scheme

	kube      client.Client
	installer *installer
}

func NewOpenFSCPlugin() (*OpenFSCPlugin, error) {
	var cfg pluginConfig
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("parse plugin config: %w", err)
	}
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		clientgoscheme.AddToScheme,
		apiextensionsv1.AddToScheme,
		openfscv1.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			return nil, fmt.Errorf("build scheme: %w", err)
		}
	}
	return &OpenFSCPlugin{cfg: cfg, scheme: scheme}, nil
}

// init runs lazily because both Start and the Installer lifecycle methods can
// be the first caller.
func (p *OpenFSCPlugin) init(host pluginruntime.Host) error {
	if p.kube == nil {
		c, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: p.scheme})
		if err != nil {
			return fmt.Errorf("create kubernetes client: %w", err)
		}
		p.kube = c
	}
	if p.installer == nil {
		p.installer = newInstaller(&p.cfg, p.kube, host.Logger())
	}
	return nil
}

func (p *OpenFSCPlugin) Start(ctx context.Context, host pluginruntime.Host) error {
	if err := p.init(host); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("init: %w", pluginerrors.NewPermanent(err))
	}

	installed, err := p.installer.isInstalled(ctx)
	if err != nil {
		return fmt.Errorf("check openfsc-operator status: %w", pluginerrors.NewTransient(err))
	}
	if !installed {
		if err := p.Install(ctx, host); err != nil {
			host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
			return fmt.Errorf("install OpenFSC operator: %w", pluginerrors.NewTransient(err))
		}
	}

	host.ReportReady()
	p.reportInstallationsStatus(ctx, host)

	<-ctx.Done()
	return nil
}

func (p *OpenFSCPlugin) Shutdown(_ context.Context) error {
	return nil
}

func (p *OpenFSCPlugin) Install(ctx context.Context, host pluginruntime.Host) error {
	if err := p.init(host); err != nil {
		return fmt.Errorf("init: %w", err)
	}
	return p.installer.install(ctx, host)
}

func (p *OpenFSCPlugin) Uninstall(ctx context.Context, host pluginruntime.Host) error {
	if err := p.init(host); err != nil {
		return fmt.Errorf("init: %w", err)
	}
	return p.installer.uninstall(ctx)
}

func (p *OpenFSCPlugin) Upgrade(ctx context.Context, host pluginruntime.Host) error {
	return p.Install(ctx, host)
}

func (p *OpenFSCPlugin) Reconcile(ctx context.Context, host pluginruntime.Host) error {
	if p.kube == nil {
		return nil
	}
	if err := crd.VerifyAll(ctx, p.kube, crdNames); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("reconcile: CRDs missing: %w", pluginerrors.NewTransient(err))
	}
	p.reportInstallationsStatus(ctx, host)
	return nil
}

// reportInstallationsStatus summarizes the cluster's FSCInstallations in the
// plugin status. The plugin is a capability provider: it stays Running with
// zero installations, and per-installation health lives on the resources.
func (p *OpenFSCPlugin) reportInstallationsStatus(ctx context.Context, host pluginruntime.Host) {
	var list openfscv1.FSCInstallationList
	if err := p.kube.List(ctx, &list); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return
	}
	if len(list.Items) == 0 {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: "openfsc-operator running; no FSCInstallations declared yet"})
		return
	}

	var active, pending, errored int
	for i := range list.Items {
		switch list.Items[i].Status.Phase {
		case openfscv1.PhaseActive:
			active++
		case openfscv1.PhaseError:
			errored++
		default:
			pending++
		}
	}
	host.ReportStatus(pluginruntime.PluginStatus{
		Phase:   pluginruntime.PhaseRunning,
		Message: fmt.Sprintf("%d installations: %d active, %d pending, %d error", len(list.Items), active, pending, errored),
	})
}
