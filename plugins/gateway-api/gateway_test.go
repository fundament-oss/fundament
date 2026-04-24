package main

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestBuildDefaultGateway(t *testing.T) {
	cfg := pluginConfig{
		GatewayName:      "fundament-gateway",
		GatewayNamespace: "istio-system",
	}

	raw := buildDefaultGateway(cfg, false)

	var parsed map[string]any
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("invalid YAML: %v", err)
	}

	if parsed["kind"] != "Gateway" {
		t.Fatalf("expected kind Gateway, got %v", parsed["kind"])
	}

	metadata := parsed["metadata"].(map[string]any)
	if metadata["name"] != "fundament-gateway" {
		t.Fatalf("expected name fundament-gateway, got %v", metadata["name"])
	}
	if metadata["namespace"] != "istio-system" {
		t.Fatalf("expected namespace istio-system, got %v", metadata["namespace"])
	}

	spec := parsed["spec"].(map[string]any)
	if spec["gatewayClassName"] != "istio" {
		t.Fatalf("expected gatewayClassName istio, got %v", spec["gatewayClassName"])
	}

	listeners := spec["listeners"].([]any)
	if len(listeners) != 2 {
		t.Fatalf("expected 2 listeners, got %d", len(listeners))
	}
}

func TestBuildDefaultGatewayWithCertManager(t *testing.T) {
	cfg := pluginConfig{
		GatewayName:      "fundament-gateway",
		GatewayNamespace: "istio-system",
	}

	raw := buildDefaultGateway(cfg, true)

	var parsed map[string]any
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("invalid YAML: %v", err)
	}

	metadata := parsed["metadata"].(map[string]any)
	annotations := metadata["annotations"].(map[string]any)
	if _, ok := annotations["cert-manager.io/cluster-issuer"]; !ok {
		t.Fatal("expected cert-manager annotation when certManagerAvailable is true")
	}
}

func TestBuildDefaultGatewayWithoutCertManager(t *testing.T) {
	cfg := pluginConfig{
		GatewayName:      "fundament-gateway",
		GatewayNamespace: "istio-system",
	}

	raw := buildDefaultGateway(cfg, false)

	var parsed map[string]any
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("invalid YAML: %v", err)
	}

	metadata := parsed["metadata"].(map[string]any)
	if metadata["annotations"] != nil {
		t.Fatal("expected no annotations when certManagerAvailable is false")
	}
}
