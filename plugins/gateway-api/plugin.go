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

var certManagerCRDs = []string{
	"certificates.cert-manager.io",
	"clusterissuers.cert-manager.io",
}

// GatewayAPIPlugin implements the Gateway API Fundament plugin powered by Istio.
type GatewayAPIPlugin struct {
	def       pluginruntime.PluginDefinition
	cfg       pluginConfig
	istio     *istioInstaller
	k8sClient client.Client

	certManagerAvailable bool
}

// NewGatewayAPIPlugin creates a new GatewayAPIPlugin with the given definition.
func NewGatewayAPIPlugin(def *pluginruntime.PluginDefinition) (*GatewayAPIPlugin, error) {
	var cfg pluginConfig
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("parse plugin config: %w", err)
	}

	return &GatewayAPIPlugin{
		def:   *def,
		cfg:   cfg,
		istio: newIstioInstaller(cfg),
	}, nil
}

func (p *GatewayAPIPlugin) Definition() pluginruntime.PluginDefinition {
	return p.def
}

func (p *GatewayAPIPlugin) Start(ctx context.Context, host pluginruntime.Host) error {
	installed, err := p.istio.isInstalled(ctx)
	if err != nil {
		return fmt.Errorf("check istio status: %w", pluginerrors.NewTransient(err))
	}

	if !installed {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseInstalling, Message: "installing Istio"})
		if err := p.Install(ctx, host); err != nil {
			host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
			return fmt.Errorf("install istio: %w", pluginerrors.NewTransient(err))
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

	if err := crd.VerifyAll(ctx, p.k8sClient, gatewayAPICRDs); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("verify CRDs: %w", pluginerrors.NewTransient(err))
	}

	p.certManagerAvailable = p.detectCertManager(ctx)

	if err := p.ensureDefaultGateway(ctx); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("ensure default gateway: %w", pluginerrors.NewTransient(err))
	}

	host.ReportReady()
	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: "Gateway API is running"})

	<-ctx.Done()
	return nil
}

func (p *GatewayAPIPlugin) Shutdown(_ context.Context) error {
	return nil
}

func (p *GatewayAPIPlugin) Install(ctx context.Context, _ pluginruntime.Host) error {
	if err := p.istio.install(ctx); err != nil {
		return fmt.Errorf("install istio: %w", err)
	}
	return nil
}

func (p *GatewayAPIPlugin) Uninstall(ctx context.Context, host pluginruntime.Host) error {
	if p.k8sClient != nil {
		remaining, err := p.listUserResources(ctx)
		if err != nil {
			return fmt.Errorf("check user resources: %w", err)
		}
		if len(remaining) > 0 {
			return fmt.Errorf("cannot uninstall: %d user-created Gateway/Route resources still exist — remove them first", len(remaining))
		}

		if err := p.deleteDefaultGateway(ctx); err != nil {
			host.Logger().Warn("failed to delete default gateway during uninstall", "error", err)
		}
	}

	if err := p.istio.uninstall(ctx); err != nil {
		return fmt.Errorf("uninstall istio: %w", err)
	}
	return nil
}

func (p *GatewayAPIPlugin) Upgrade(ctx context.Context, host pluginruntime.Host) error {
	return p.Install(ctx, host)
}

func (p *GatewayAPIPlugin) Reconcile(ctx context.Context, host pluginruntime.Host) error {
	if p.k8sClient == nil {
		return nil
	}

	if err := crd.VerifyAll(ctx, p.k8sClient, gatewayAPICRDs); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("reconcile: CRDs missing: %w", pluginerrors.NewTransient(err))
	}

	if !p.isIstiodHealthy(ctx) {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: "istiod control plane is unhealthy"})
		return fmt.Errorf("reconcile: istiod unhealthy: %w", pluginerrors.NewTransient(fmt.Errorf("istiod not ready")))
	}

	if err := p.ensureDefaultGateway(ctx); err != nil {
		host.Logger().Warn("reconcile: failed to ensure default gateway", "error", err)
	}

	p.certManagerAvailable = p.detectCertManager(ctx)

	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: "Gateway API is running"})
	return nil
}

func (p *GatewayAPIPlugin) detectCertManager(ctx context.Context) bool {
	for _, name := range certManagerCRDs {
		ok, err := crd.Exists(ctx, p.k8sClient, name)
		if err != nil || !ok {
			return false
		}
	}
	return true
}

func (p *GatewayAPIPlugin) ensureDefaultGateway(ctx context.Context) error {
	gw := &unstructured.Unstructured{}
	gw.SetGroupVersionKind(gatewayGVK())
	err := p.k8sClient.Get(ctx, types.NamespacedName{
		Name:      p.cfg.GatewayName,
		Namespace: p.cfg.GatewayNamespace,
	}, gw)
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("get default gateway: %w", err)
	}

	raw := buildDefaultGateway(p.cfg, p.certManagerAvailable)
	obj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal(raw, &obj.Object); err != nil {
		return fmt.Errorf("parse default gateway: %w", err)
	}

	if err := p.k8sClient.Create(ctx, obj); err != nil {
		return fmt.Errorf("create default gateway: %w", err)
	}
	return nil
}

func (p *GatewayAPIPlugin) deleteDefaultGateway(ctx context.Context) error {
	gw := &unstructured.Unstructured{}
	gw.SetGroupVersionKind(gatewayGVK())
	gw.SetName(p.cfg.GatewayName)
	gw.SetNamespace(p.cfg.GatewayNamespace)
	if err := p.k8sClient.Delete(ctx, gw); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("delete default gateway: %w", err)
	}
	return nil
}

func (p *GatewayAPIPlugin) isIstiodHealthy(ctx context.Context) bool {
	deploy := &unstructured.Unstructured{}
	deploy.SetGroupVersionKind(deploymentGVK())
	err := p.k8sClient.Get(ctx, types.NamespacedName{
		Name:      "istiod",
		Namespace: p.cfg.GatewayNamespace,
	}, deploy)
	if err != nil {
		return false
	}

	status, ok := deploy.Object["status"].(map[string]any)
	if !ok {
		return false
	}
	available, _ := status["availableReplicas"].(int64)
	return available > 0
}

func (p *GatewayAPIPlugin) listUserResources(ctx context.Context) ([]string, error) {
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
		list.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   g.gvk.Group,
			Version: g.gvk.Version,
			Kind:    g.listKind,
		})
		if err := p.k8sClient.List(ctx, list); err != nil {
			continue
		}
		for _, item := range list.Items {
			if item.GetKind() == "Gateway" &&
				item.GetName() == p.cfg.GatewayName &&
				item.GetNamespace() == p.cfg.GatewayNamespace {
				continue
			}
			resources = append(resources, fmt.Sprintf("%s/%s/%s", item.GetKind(), item.GetNamespace(), item.GetName()))
		}
	}
	return resources, nil
}

func gatewayGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "gateway.networking.k8s.io",
		Version: "v1",
		Kind:    "Gateway",
	}
}

func deploymentGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}
}
