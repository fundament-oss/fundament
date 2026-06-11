package main

// pluginConfig holds OpenFSC plugin configuration from FUNP_* env vars.
//
// All fields have defaults so the plugin installs a self-contained, single-peer
// directory with zero configuration. The fields map onto the Directory resource
// the plugin creates; everything else (admin API addresses, certificates,
// Postgres) is derived by the openfsc-operator from the Directory.
type pluginConfig struct {
	// GroupID is the FSC group the directory peer belongs to.
	GroupID string `env:"FUNP_GROUP_ID" envDefault:"fsc-demo"`
	// DirectoryPeerID is the peer ID of the directory (the subject serialNumber on
	// its group certificate).
	DirectoryPeerID string `env:"FUNP_DIRECTORY_PEER_ID" envDefault:"12345678901234567899"`
	// Namespace is where the OpenFSC Manager/Controller are installed.
	Namespace string `env:"FUNP_FSC_NAMESPACE" envDefault:"fsc"`
	// ControllerURL is the host-reachable URL of the Controller UI, surfaced on
	// the directory peer so the console can link users to it. The in-cluster
	// service DNS does not resolve from a developer's host, so this defaults to
	// the local port-forward target; set it to an ingress host in real
	// deployments. Empty hides the link.
	ControllerURL string `env:"FUNP_CONTROLLER_URL" envDefault:"http://localhost:9080"`
	// OperatorImage is the openfsc-operator image the plugin's vendored operator
	// chart deploys. The sandbox flow overrides it with a locally-pushed image
	// (see Justfile recipe operator-push).
	OperatorImage string `env:"FUNP_OPERATOR_IMAGE" envDefault:"ghcr.io/fundament-oss/fundament/openfsc-operator:latest"`
}
