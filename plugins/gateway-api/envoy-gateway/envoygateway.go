package main

import (
	"context"
	"fmt"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/helm"
)

const (
	envoyGatewayReleaseName = "eg"
	envoyGatewayChartRef    = "oci://docker.io/envoyproxy/gateway-helm"
)

// chartSpec is the single Helm OCI release that installs the Envoy Gateway
// controller (and, bundled with it, the Gateway API + Envoy CRDs).
type chartSpec struct {
	releaseName string
	chartRef    string
	version     string
}

type envoyGatewayInstaller struct {
	cfg        pluginConfig
	helmClient *helm.Client
}

func newEnvoyGatewayInstaller(cfg pluginConfig) *envoyGatewayInstaller {
	return &envoyGatewayInstaller{
		cfg:        cfg,
		helmClient: helm.NewClient(cfg.GatewayNamespace),
	}
}

func (i *envoyGatewayInstaller) chart() chartSpec {
	return chartSpec{
		releaseName: envoyGatewayReleaseName,
		chartRef:    envoyGatewayChartRef,
		version:     i.cfg.EnvoyGatewayVersion,
	}
}

func (i *envoyGatewayInstaller) install(ctx context.Context) error {
	c := i.chart()
	if err := i.helmClient.InstallFromOCI(ctx, c.releaseName, c.chartRef, c.version, nil); err != nil {
		return fmt.Errorf("install %s: %w", c.releaseName, err)
	}
	return nil
}

func (i *envoyGatewayInstaller) uninstall(ctx context.Context) error {
	if err := i.helmClient.Uninstall(ctx, envoyGatewayReleaseName); err != nil {
		return fmt.Errorf("uninstall %s: %w", envoyGatewayReleaseName, err)
	}
	return nil
}

func (i *envoyGatewayInstaller) isInstalled(ctx context.Context) (bool, error) {
	return i.helmClient.IsInstalled(ctx, envoyGatewayReleaseName)
}
