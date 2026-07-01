package config

import (
	"fmt"
	"log/slog"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	ListenAddr         string     `env:"LISTEN_ADDR" envDefault:":8080"`
	InternalListenAddr string     `env:"INTERNAL_LISTEN_ADDR" envDefault:":8081"`
	LogLevel           slog.Level `env:"LOG_LEVEL" envDefault:"info"`
	Mode               string     `env:"PLUGIN_PROXY_MODE" envDefault:"mock"`

	// JWTSecret signs and verifies PluginTokens. Required.
	JWTSecret string `env:"JWT_SECRET,required,notEmpty"`

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
	default:
		return Config{}, fmt.Errorf("PLUGIN_PROXY_MODE=%q: only %q or %q is supported", cfg.Mode, "mock", "real")
	}
	return cfg, nil
}
