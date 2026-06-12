package main

// pluginConfig holds configuration from FUNP_* env vars; everything an
// installation needs lives on the FSCInstallation resources instead.
type pluginConfig struct {
	// OperatorImage is the openfsc-operator image the plugin's vendored operator
	// chart deploys. The sandbox flow overrides it with a locally-pushed image
	// (see Justfile recipe operator-push).
	OperatorImage string `env:"FUNP_OPERATOR_IMAGE" envDefault:"ghcr.io/fundament-oss/fundament/openfsc-operator:latest"`
}
