package main

// pluginConfig holds OpenFSC plugin configuration from FUNP_* env vars.
//
// All fields have defaults so the plugin installs a self-contained, single-peer
// directory with zero configuration. The defaults match the upstream demo CA
// shipped with the OpenFSC charts (the directory peer ID is the demo CA's
// certificate serial number).
type pluginConfig struct {
	// GroupID is the FSC group the directory peer belongs to.
	GroupID string `env:"FUNP_GROUP_ID" envDefault:"fsc-demo"`
	// DirectoryPeerID is the peer ID of the directory (the demo CA serial number).
	DirectoryPeerID string `env:"FUNP_DIRECTORY_PEER_ID" envDefault:"12345678901234567899"`
	// Namespace is where the OpenFSC Manager/Controller are installed.
	Namespace string `env:"FUNP_FSC_NAMESPACE" envDefault:"fsc"`
	// ManagerAddress is the in-cluster https:// address of the directory Manager.
	ManagerAddress string `env:"FUNP_MANAGER_ADDRESS" envDefault:"https://shared-open-fsc-manager-external.fsc:8443"`
	// ControllerURL is the host-reachable URL of the Controller UI, surfaced on
	// the directory peer so the console can link users to it. The in-cluster
	// service DNS does not resolve from a developer's host, so this defaults to
	// the local port-forward target; set it to an ingress host in real
	// deployments. Empty hides the link.
	ControllerURL string `env:"FUNP_CONTROLLER_URL" envDefault:"http://localhost:9080"`

	// ControllerAdminAddress is the in-cluster https:// address of the Controller
	// Administration API (mTLS, port 9444). Empty defaults to the directory peer's
	// controller in Namespace; the Inway/Outway reconcilers use it to observe
	// registrations.
	ControllerAdminAddress string `env:"FUNP_CONTROLLER_ADMIN_ADDRESS" envDefault:""`
	// ControllerServerName is the name verified against the Controller's TLS
	// certificate. The `shared` umbrella issues the controller's internal cert with
	// the short service name as its only SAN, so dialing the cross-namespace FQDN
	// needs this override to verify (rather than skipping verification).
	ControllerServerName string `env:"FUNP_CONTROLLER_SERVER_NAME" envDefault:"shared-open-fsc-controller"`
	// FSCCertSecret is a "namespace/name" reference to the mTLS client bundle
	// (tls.crt/tls.key/ca.crt) the operator presents to the Controller
	// Administration API. Empty defaults to the controller's own internal TLS
	// Secret (shared-directory-controller-internal-tls) in Namespace, whose
	// identity the Administration API accepts; if that Secret is absent the
	// Inway/Outway reconcilers report a not-configured status instead of observing.
	FSCCertSecret string `env:"FUNP_FSC_CERT_SECRET" envDefault:""`
	// FSCInsecure skips server-certificate verification on the Administration API
	// (dev only; mTLS client auth is still enforced). Prefer ControllerServerName;
	// this is a fallback when even the short-name SAN does not match.
	FSCInsecure bool `env:"FUNP_FSC_INSECURE" envDefault:"false"`
}
