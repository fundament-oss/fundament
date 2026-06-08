package main

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/caarlos0/env/v11"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
	pluginerrors "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/errors"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/crd"
)

// valuesFundament is the Helm override layered on top of the upstream `shared`
// umbrella's values.yaml (see installer.installUmbrella).
//
//go:embed values-fundament.yaml
var valuesFundament []byte

// OpenFSCPlugin installs and supervises a self-contained OpenFSC directory peer
// (Manager + Controller, the Manager acting as the group's Directory) and owns
// the openfsc.fundament.io Peer CRD.
type OpenFSCPlugin struct {
	def    pluginruntime.PluginDefinition
	cfg    pluginConfig
	scheme *runtime.Scheme

	kube      client.Client
	installer *installer
}

// NewOpenFSCPlugin creates a new OpenFSCPlugin with the given definition.
func NewOpenFSCPlugin(def *pluginruntime.PluginDefinition) (*OpenFSCPlugin, error) {
	var cfg pluginConfig
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("parse plugin config: %w", err)
	}
	scheme, err := buildScheme()
	if err != nil {
		return nil, fmt.Errorf("build scheme: %w", err)
	}
	return &OpenFSCPlugin{def: *def, cfg: cfg, scheme: scheme}, nil
}

func (p *OpenFSCPlugin) Definition() pluginruntime.PluginDefinition {
	return p.def
}

// init lazily builds the Kubernetes client and installer (both Start and the
// Installer lifecycle methods need them).
func (p *OpenFSCPlugin) init(host pluginruntime.Host) error {
	if p.kube == nil {
		c, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: p.scheme})
		if err != nil {
			return fmt.Errorf("create kubernetes client: %w", err)
		}
		p.kube = c
	}
	if p.installer == nil {
		p.installer = newInstaller(p.cfg, p.kube, host.Logger())
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
		return fmt.Errorf("check OpenFSC status: %w", pluginerrors.NewTransient(err))
	}
	if !installed {
		if err := p.Install(ctx, host); err != nil {
			host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
			return fmt.Errorf("install OpenFSC: %w", pluginerrors.NewTransient(err))
		}
	}

	// Run the operator: apply the Peer CRD, seed the self peer and reconcile.
	// Blocks until ctx is cancelled.
	return p.runOperator(ctx, host)
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
	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: "OpenFSC directory running"})
	return nil
}
