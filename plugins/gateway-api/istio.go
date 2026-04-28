package main

import (
	"context"
	"fmt"
	"slices"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/helm"
)

const (
	istioRepoURL = "https://istio-release.storage.googleapis.com/charts"
)

type chartRelease struct {
	releaseName string
	chartName   string
	values      map[string]string
}

type istioInstaller struct {
	cfg        pluginConfig
	helmClient *helm.Client
}

func newIstioInstaller(cfg pluginConfig) *istioInstaller {
	return &istioInstaller{
		cfg:        cfg,
		helmClient: helm.NewClient(cfg.GatewayNamespace),
	}
}

func (i *istioInstaller) installOrder() []chartRelease {
	var autoInject string
	switch i.cfg.IstioProfile {
	case "minimal":
		autoInject = "disabled"
	case "full":
		autoInject = "enabled"
	default:
		panic(fmt.Sprintf("unsupported istio profile: %q", i.cfg.IstioProfile))
	}

	return []chartRelease{
		{
			releaseName: "istio-base",
			chartName:   "base",
			values:      map[string]string{},
		},
		{
			releaseName: "istiod",
			chartName:   "istiod",
			values: map[string]string{
				"global.proxy.autoInject": autoInject,
			},
		},
		{
			releaseName: "istio-ingressgateway",
			chartName:   "gateway",
			values:      map[string]string{},
		},
	}
}

func (i *istioInstaller) uninstallOrder() []chartRelease {
	order := i.installOrder()
	slices.Reverse(order)
	return order
}

func (i *istioInstaller) install(ctx context.Context) error {
	for _, chart := range i.installOrder() {
		if err := i.helmClient.InstallFromRepo(ctx, chart.releaseName, chart.chartName, istioRepoURL, i.cfg.IstioVersion, chart.values); err != nil {
			return fmt.Errorf("install %s: %w", chart.releaseName, err)
		}
	}
	return nil
}

func (i *istioInstaller) uninstall(ctx context.Context) error {
	for _, chart := range i.uninstallOrder() {
		if err := i.helmClient.Uninstall(ctx, chart.releaseName); err != nil {
			return fmt.Errorf("uninstall %s: %w", chart.releaseName, err)
		}
	}
	return nil
}

func (i *istioInstaller) isInstalled(ctx context.Context) (bool, error) {
	return i.helmClient.IsInstalled(ctx, "istiod")
}
