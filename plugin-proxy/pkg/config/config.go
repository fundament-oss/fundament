package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

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
		// public dir. Default relative to the repo root so `just dev` works out
		// of the box; the resolveDir step below turns it into an absolute path
		// AND fails startup if the directory doesn't exist, so a mis-configured
		// cwd surfaces as a startup error instead of silent 404s on every
		// /plugins/sdk/v1/ request.
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

	if cfg.PluginSDKDir != "" {
		abs, err := resolvePluginSDKDir(cfg.PluginSDKDir)
		if err != nil {
			return Config{}, err
		}
		cfg.PluginSDKDir = abs
	}
	return cfg, nil
}

// resolvePluginSDKDir converts a relative PLUGIN_SDK_DIR into an absolute path
// and verifies that it exists as a directory. Fails loud at startup so a
// mis-configured cwd surfaces immediately rather than 404-ing every SDK asset.
func resolvePluginSDKDir(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve PLUGIN_SDK_DIR %q: %w", dir, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("PLUGIN_SDK_DIR %q not accessible: %w", abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("PLUGIN_SDK_DIR %q is not a directory", abs)
	}
	return abs, nil
}
