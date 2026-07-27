package main

// pluginConfig holds envoy-gateway plugin configuration from FUNP_* env vars.
// These are operator-level knobs only; per-Gateway configuration is done by
// users through the console CRD forms, not here.
type pluginConfig struct {
	EnvoyGatewayVersion string `env:"FUNP_ENVOY_GATEWAY_VERSION" envDefault:"v1.8.3"`
	GatewayNamespace    string `env:"FUNP_GATEWAY_NAMESPACE" envDefault:"envoy-gateway-system"`
	GatewayClassName    string `env:"FUNP_GATEWAY_CLASS_NAME" envDefault:"eg"`
}
