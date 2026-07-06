package config

import (
	"fmt"
	"log/slog"

	"github.com/caarlos0/env/v11"

	"github.com/fundament-oss/fundament/common/authz"
)

type Config struct {
	ListenAddr         string     `env:"LISTEN_ADDR" envDefault:":8080"`
	InternalListenAddr string     `env:"INTERNAL_LISTEN_ADDR" envDefault:":8081"`
	LogLevel           slog.Level `env:"LOG_LEVEL" envDefault:"info"`
	Mode               string     `env:"PLUGIN_PROXY_MODE" envDefault:"mock"`

	// JWTSecret signs and verifies PluginTokens AND validates the UserToken
	// cookie on inbound asset requests. Required.
	JWTSecret string `env:"JWT_SECRET,required,notEmpty"`

	// OpenFGA drives the asset-handler's can_view(user, cluster) gate — the
	// same check authn-api runs before minting a PluginToken.
	OpenFGA authz.Config

	// PluginProxyOrigin is this service's own public origin.
	// Required in real mode; mock-mode default applies otherwise.
	PluginProxyOrigin string `env:"PLUGIN_PROXY_ORIGIN"`
	// KubeAPIProxyOrigin feeds CSP connect-src/form-action and is the second
	// proxy origin reachable from plugin JS.
	KubeAPIProxyOrigin string `env:"KUBE_API_PROXY_ORIGIN"`
	// ConsoleOrigin is the only origin permitted to embed the iframe
	// (CSP frame-ancestors).
	ConsoleOrigin string `env:"CONSOLE_ORIGIN"`

	// PluginSDKDir is the local directory served at /plugins/sdk/v1/ — it holds
	// the built sdk.js/sdk.css that plugin HTML loads via <script src>. Empty
	// disables the route (the plugin CSP is script-src 'self', so plugins
	// cannot fetch the SDK from elsewhere).
	//
	// The /v1/ segment matches fundament:init's protocolVersion. A future
	// protocol bump lands as /plugins/sdk/v2/ alongside, so plugins pinned to
	// v1 keep working.
	PluginSDKDir string `env:"PLUGIN_SDK_DIR"`

	// PluginSandboxKubeconfig is a filesystem path to a kubeconfig that grants
	// admin access to a locally-running plugin sandbox cluster (e.g. exported
	// from `k3d kubeconfig get fundament-plugin`). When set, plugin-proxy
	// uses PodFetcher against that cluster for asset fetches — pins every
	// clusterID → this kubeconfig. Empty falls back to MockFetcher (mock mode)
	// or the stubbed Gardener path (real mode).
	PluginSandboxKubeconfig string `env:"PLUGIN_SANDBOX_KUBECONFIG"`
}

// FromEnv parses env vars and applies mode-specific defaults/validation.
// In mock mode, origin fields default to fundament.localhost dev hosts.
// In real mode, all three origin fields are required.
func FromEnv() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("env parse: %w", err)
	}

	switch cfg.Mode {
	case "mock":
		if cfg.PluginProxyOrigin == "" {
			cfg.PluginProxyOrigin = "http://plugin-proxy.fundament.localhost:8080"
		}
		if cfg.KubeAPIProxyOrigin == "" {
			cfg.KubeAPIProxyOrigin = "http://kube-api-proxy.fundament.localhost:8080"
		}
		if cfg.ConsoleOrigin == "" {
			cfg.ConsoleOrigin = "http://console.fundament.localhost:8080"
		}
		// Local dev: the plugin-sdk build output lives in the console-frontend
		// public dir. Runs from the repo root.
		if cfg.PluginSDKDir == "" {
			cfg.PluginSDKDir = "console-frontend/public/plugin-ui"
		}
	case "real":
		if cfg.PluginProxyOrigin == "" || cfg.KubeAPIProxyOrigin == "" || cfg.ConsoleOrigin == "" {
			return Config{}, fmt.Errorf("PLUGIN_PROXY_ORIGIN, KUBE_API_PROXY_ORIGIN, and CONSOLE_ORIGIN are required in real mode")
		}
	default:
		return Config{}, fmt.Errorf("PLUGIN_PROXY_MODE=%q: only %q or %q is supported", cfg.Mode, "mock", "real")
	}
	return cfg, nil
}
