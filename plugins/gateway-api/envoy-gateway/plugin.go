package main

import (
	"context"
	"fmt"

	"github.com/caarlos0/env/v11"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
	pluginerrors "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/errors"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/crd"
)

var gatewayAPICRDs = []string{
	"gateways.gateway.networking.k8s.io",
	"httproutes.gateway.networking.k8s.io",
	"grpcroutes.gateway.networking.k8s.io",
	"tcproutes.gateway.networking.k8s.io",
	"tlsroutes.gateway.networking.k8s.io",
}

var envoyGatewayCRDs = []string{
	"envoyproxies.gateway.envoyproxy.io",
	"securitypolicies.gateway.envoyproxy.io",
	"backendtrafficpolicies.gateway.envoyproxy.io",
	"clienttrafficpolicies.gateway.envoyproxy.io",
}

// verifiedCRDs is the full set the plugin checks for after install — the Envoy
// Gateway chart bundles all of them.
func verifiedCRDs() []string {
	return append(append([]string{}, gatewayAPICRDs...), envoyGatewayCRDs...)
}

// EnvoyGatewayPlugin installs and runs the Envoy Gateway platform. Users create
// Gateways/Routes/policies through the console CRD forms; the plugin never
// creates a default Gateway.
type EnvoyGatewayPlugin struct {
	cfg       pluginConfig
	installer *envoyGatewayInstaller
	k8sClient client.Client
}

func NewEnvoyGatewayPlugin() (*EnvoyGatewayPlugin, error) {
	var cfg pluginConfig
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("parse plugin config: %w", err)
	}
	return &EnvoyGatewayPlugin{cfg: cfg, installer: newEnvoyGatewayInstaller(cfg)}, nil
}

func (p *EnvoyGatewayPlugin) Start(ctx context.Context, host pluginruntime.Host) error {
	cfg := ctrl.GetConfigOrDie()

	// Preflight the cluster version before touching Helm: on Kubernetes < 1.31 the
	// bundled TLSRoute CRD is rejected as invalid, so surface a clear message here
	// instead of a cryptic "missing CRDs: [tlsroutes...]" later in the install.
	discoveryClient, err := newDiscoveryClient(cfg)
	if err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("create discovery client: %w", pluginerrors.NewPermanent(err))
	}
	if err := checkKubernetesVersion(discoveryClient, p.cfg.EnvoyGatewayVersion); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("kubernetes preflight: %w", pluginerrors.NewTransient(err))
	}

	installed, err := p.installer.isInstalled(ctx)
	if err != nil {
		return fmt.Errorf("check envoy gateway status: %w", pluginerrors.NewTransient(err))
	}
	if !installed {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseInstalling, Message: "installing Envoy Gateway"})
		if err := p.Install(ctx, host); err != nil {
			host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
			return fmt.Errorf("install envoy gateway: %w", pluginerrors.NewTransient(err))
		}
	}

	scheme := runtime.NewScheme()
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("add apiextensions to scheme: %w", pluginerrors.NewPermanent(err))
	}

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("create kubernetes client: %w", pluginerrors.NewPermanent(err))
	}
	p.k8sClient = k8sClient

	if err := crd.VerifyAll(ctx, p.k8sClient, verifiedCRDs()); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("verify CRDs: %w", pluginerrors.NewTransient(err))
	}

	if err := p.ensureGatewayClass(ctx); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("ensure gateway class: %w", pluginerrors.NewTransient(err))
	}

	host.ReportReady()
	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: "Envoy Gateway is running"})

	<-ctx.Done()
	return nil
}

func (p *EnvoyGatewayPlugin) Shutdown(_ context.Context) error { return nil }

func (p *EnvoyGatewayPlugin) Install(ctx context.Context, _ pluginruntime.Host) error {
	if err := p.installer.install(ctx); err != nil {
		return fmt.Errorf("install envoy gateway: %w", err)
	}
	return nil
}

func (p *EnvoyGatewayPlugin) Uninstall(ctx context.Context, host pluginruntime.Host) error {
	if p.k8sClient != nil {
		remaining, err := p.listUserResources(ctx)
		if err != nil {
			return fmt.Errorf("check user resources: %w", err)
		}
		if len(remaining) > 0 {
			return fmt.Errorf("cannot uninstall: %d user-created Gateway/Route resources still exist — remove them first", len(remaining))
		}
		if err := p.deleteGatewayClass(ctx); err != nil {
			host.Logger().Warn("failed to delete gateway class during uninstall", "error", err)
		}
	}
	if err := p.installer.uninstall(ctx); err != nil {
		return fmt.Errorf("uninstall envoy gateway: %w", err)
	}
	return nil
}

func (p *EnvoyGatewayPlugin) Upgrade(ctx context.Context, host pluginruntime.Host) error {
	return p.Install(ctx, host)
}

func (p *EnvoyGatewayPlugin) Reconcile(ctx context.Context, host pluginruntime.Host) error {
	if p.k8sClient == nil {
		return nil
	}
	if err := crd.VerifyAll(ctx, p.k8sClient, verifiedCRDs()); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("reconcile: CRDs missing: %w", pluginerrors.NewTransient(err))
	}
	if !p.isControllerHealthy(ctx) {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: "envoy-gateway control plane is unhealthy"})
		return fmt.Errorf("reconcile: envoy-gateway unhealthy: %w", pluginerrors.NewTransient(fmt.Errorf("envoy-gateway not ready")))
	}
	if err := p.ensureGatewayClass(ctx); err != nil {
		host.Logger().Warn("reconcile: failed to ensure gateway class", "error", err)
	}
	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: "Envoy Gateway is running"})
	return nil
}

func (p *EnvoyGatewayPlugin) ensureGatewayClass(ctx context.Context) error {
	gc := &unstructured.Unstructured{}
	gc.SetGroupVersionKind(gatewayClassGVK())
	err := p.k8sClient.Get(ctx, types.NamespacedName{Name: p.cfg.GatewayClassName}, gc)
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("get gateway class: %w", err)
	}

	obj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal(buildGatewayClass(p.cfg), &obj.Object); err != nil {
		return fmt.Errorf("parse gateway class: %w", err)
	}
	if err := p.k8sClient.Create(ctx, obj); err != nil {
		return fmt.Errorf("create gateway class: %w", err)
	}
	return nil
}

func (p *EnvoyGatewayPlugin) deleteGatewayClass(ctx context.Context) error {
	gc := &unstructured.Unstructured{}
	gc.SetGroupVersionKind(gatewayClassGVK())
	gc.SetName(p.cfg.GatewayClassName)
	if err := p.k8sClient.Delete(ctx, gc); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("delete gateway class: %w", err)
	}
	return nil
}

func (p *EnvoyGatewayPlugin) isControllerHealthy(ctx context.Context) bool {
	deploy := &unstructured.Unstructured{}
	deploy.SetGroupVersionKind(deploymentGVK())
	if err := p.k8sClient.Get(ctx, types.NamespacedName{Name: "envoy-gateway", Namespace: p.cfg.GatewayNamespace}, deploy); err != nil {
		return false
	}
	status, ok := deploy.Object["status"].(map[string]any)
	if !ok {
		return false
	}
	available, _ := status["availableReplicas"].(float64)
	return available > 0
}

// listUserResources returns user-created Gateways/Routes that block uninstall.
// Unlike the Istio plugin there is no default Gateway to exclude — any Gateway
// or Route counts.
func (p *EnvoyGatewayPlugin) listUserResources(ctx context.Context) ([]string, error) {
	var resources []string
	gvks := []struct {
		gvk      schema.GroupVersionKind
		listKind string
	}{
		{schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway"}, "GatewayList"},
		{schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute"}, "HTTPRouteList"},
		{schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "GRPCRoute"}, "GRPCRouteList"},
		{schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1alpha2", Kind: "TCPRoute"}, "TCPRouteList"},
		{schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1alpha2", Kind: "TLSRoute"}, "TLSRouteList"},
	}
	for _, g := range gvks {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(schema.GroupVersionKind{Group: g.gvk.Group, Version: g.gvk.Version, Kind: g.listKind})
		if err := p.k8sClient.List(ctx, list); err != nil {
			return nil, fmt.Errorf("list %s: %w", g.gvk.Kind, err)
		}
		for _, item := range list.Items {
			resources = append(resources, fmt.Sprintf("%s/%s/%s", item.GetKind(), item.GetNamespace(), item.GetName()))
		}
	}
	return resources, nil
}
