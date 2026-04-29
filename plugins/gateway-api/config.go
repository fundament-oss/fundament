package main

// pluginConfig holds gateway-api plugin configuration from FUNP_* env vars.
type pluginConfig struct {
	IstioProfile     string `env:"FUNP_ISTIO_PROFILE" envDefault:"minimal"`
	IstioVersion     string `env:"FUNP_ISTIO_VERSION" envDefault:"1.26.0"`
	GatewayName      string `env:"FUNP_GATEWAY_NAME" envDefault:"fundament-gateway"`
	GatewayNamespace string `env:"FUNP_GATEWAY_NAMESPACE" envDefault:"istio-system"`
}
