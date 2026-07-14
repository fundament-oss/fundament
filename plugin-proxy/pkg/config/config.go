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

	// JWTSecret signs and verifies PluginTokens. Required.
	JWTSecret string `env:"JWT_SECRET,required,notEmpty"`

	// GardenerKubeconfig points at the garden-cluster kubeconfig used to
	// resolve shoots and mint admin kubeconfigs. Required in real mode.
	GardenerKubeconfig string `env:"GARDENER_KUBECONFIG"`

	// OpenFGA configures the can_view check on the installation routes.
	// Parsed (and required) only in real mode — its fields carry required
	// env tags that must not constrain mock deployments.
	OpenFGA authz.Config `env:"-"`

	// PluginProxyOrigin is this service's own public origin.
	// Required in real mode; mock-mode default applies otherwise.
	PluginProxyOrigin string `env:"PLUGIN_PROXY_ORIGIN"`
	// KubeAPIProxyOrigin feeds CSP connect-src/form-action and is the second
	// proxy origin reachable from plugin JS.
	KubeAPIProxyOrigin string `env:"KUBE_API_PROXY_ORIGIN"`
	// ConsoleOrigin is the only origin permitted to embed the iframe
	// (CSP frame-ancestors).
	ConsoleOrigin string `env:"CONSOLE_ORIGIN"`
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
	case "real":
		if cfg.PluginProxyOrigin == "" || cfg.KubeAPIProxyOrigin == "" || cfg.ConsoleOrigin == "" {
			return Config{}, fmt.Errorf("PLUGIN_PROXY_ORIGIN, KUBE_API_PROXY_ORIGIN, and CONSOLE_ORIGIN are required in real mode")
		}
		if cfg.GardenerKubeconfig == "" {
			return Config{}, fmt.Errorf("GARDENER_KUBECONFIG is required in real mode")
		}
		if err := env.Parse(&cfg.OpenFGA); err != nil {
			return Config{}, fmt.Errorf("openfga env parse: %w", err)
		}
	default:
		return Config{}, fmt.Errorf("PLUGIN_PROXY_MODE=%q: only %q or %q is supported", cfg.Mode, "mock", "real")
	}
	return cfg, nil
}
