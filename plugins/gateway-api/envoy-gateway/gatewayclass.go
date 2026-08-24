package main

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// envoyControllerName is the controllerName Envoy Gateway watches for; a
// GatewayClass carrying it binds the class to the installed controller.
const envoyControllerName = "gateway.envoyproxy.io/gatewayclass-controller"

const gatewayClassTemplate = `apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: %s
spec:
  controllerName: %s
`

// buildGatewayClass renders the cluster-scoped GatewayClass that binds the
// configured class name to the Envoy Gateway controller. No parametersRef:
// data-plane infra (service type, replicas) is configured per-need by users via
// EnvoyProxy / Gateway.spec.infrastructure, not baked into the class.
func buildGatewayClass(cfg pluginConfig) []byte {
	return fmt.Appendf(nil, gatewayClassTemplate, cfg.GatewayClassName, envoyControllerName)
}

func gatewayClassGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "GatewayClass"}
}

func deploymentGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
}
