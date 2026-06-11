package main

import (
	"context"
	"fmt"

	"github.com/caarlos0/env/v11"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openfscv1 "github.com/fundament-oss/fundament/openfsc-operator/pkg/api/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
	pluginerrors "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/errors"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/crd"
)

// OpenFSCPlugin is a thin installer around the standalone openfsc-operator: it
// installs the operator (plus the prerequisites the operator preflights but
// never installs itself), declares the directory as a Directory resource and
// surfaces the operator-reported status in the console. All reconciliation —
// the OpenFSC core, the self Peer, the Inway/Outway gateways — lives in the
// operator, so it keeps running when this plugin pod is down.
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
			return fmt.Errorf("install OpenFSC: %w", pluginerrors.NewTransient(err))
		}
	}

	host.ReportReady()
	p.reportDirectoryStatus(ctx, host)

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
	p.reportDirectoryStatus(ctx, host)
	return nil
}

// reportDirectoryStatus maps the operator-reported Directory status onto the
// plugin status the console shows.
func (p *OpenFSCPlugin) reportDirectoryStatus(ctx context.Context, host pluginruntime.Host) {
	var dir openfscv1.Directory
	err := p.kube.Get(ctx, types.NamespacedName{Name: directoryName}, &dir)
	if apierrors.IsNotFound(err) {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: fmt.Sprintf("Directory %q not found", directoryName)})
		return
	}
	if err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return
	}

	switch dir.Status.Phase {
	case openfscv1.PhaseActive:
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: "OpenFSC directory running"})
	case openfscv1.PhaseError:
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: dir.Status.Message})
	case openfscv1.PhasePending:
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseInstalling, Message: dir.Status.Message})
	default:
		// Phase not yet reported by the operator (e.g. it is still starting).
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseInstalling, Message: "waiting for the openfsc-operator to report Directory status"})
	}
}
