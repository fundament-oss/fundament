package main

import (
	"testing"
)

func TestIstioInstallerChartOrder(t *testing.T) {
	cfg := pluginConfig{
		IstioProfile:     "minimal",
		IstioVersion:     "1.26.0",
		GatewayNamespace: "istio-system",
	}
	installer := newIstioInstaller(cfg)

	charts := installer.installOrder()
	if len(charts) != 3 {
		t.Fatalf("expected 3 charts, got %d", len(charts))
	}
	if charts[0].releaseName != "istio-base" {
		t.Fatalf("expected first chart to be istio-base, got %s", charts[0].releaseName)
	}
	if charts[1].releaseName != "istiod" {
		t.Fatalf("expected second chart to be istiod, got %s", charts[1].releaseName)
	}
	if charts[2].releaseName != "istio-ingressgateway" {
		t.Fatalf("expected third chart to be istio-ingressgateway, got %s", charts[2].releaseName)
	}
}

func TestIstioInstallerUninstallOrder(t *testing.T) {
	cfg := pluginConfig{
		IstioProfile:     "minimal",
		IstioVersion:     "1.26.0",
		GatewayNamespace: "istio-system",
	}
	installer := newIstioInstaller(cfg)

	charts := installer.uninstallOrder()
	if len(charts) != 3 {
		t.Fatalf("expected 3 charts, got %d", len(charts))
	}
	if charts[0].releaseName != "istio-ingressgateway" {
		t.Fatalf("expected first uninstall to be istio-ingressgateway, got %s", charts[0].releaseName)
	}
	if charts[1].releaseName != "istiod" {
		t.Fatalf("expected second uninstall to be istiod, got %s", charts[1].releaseName)
	}
	if charts[2].releaseName != "istio-base" {
		t.Fatalf("expected third uninstall to be istio-base, got %s", charts[2].releaseName)
	}
}

func TestIstioInstallerFullProfileValues(t *testing.T) {
	cfg := pluginConfig{
		IstioProfile:     "full",
		IstioVersion:     "1.26.0",
		GatewayNamespace: "istio-system",
	}
	installer := newIstioInstaller(cfg)

	charts := installer.installOrder()
	istiod := charts[1]
	if istiod.values["global.proxy.autoInject"] != "enabled" {
		t.Fatal("expected sidecar injection enabled for full profile")
	}
}

func TestIstioInstallerMinimalProfileValues(t *testing.T) {
	cfg := pluginConfig{
		IstioProfile:     "minimal",
		IstioVersion:     "1.26.0",
		GatewayNamespace: "istio-system",
	}
	installer := newIstioInstaller(cfg)

	charts := installer.installOrder()
	istiod := charts[1]
	if istiod.values["global.proxy.autoInject"] != "disabled" {
		t.Fatal("expected sidecar injection disabled for minimal profile")
	}
}
