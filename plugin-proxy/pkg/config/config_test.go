package config

import (
	"strings"
	"testing"
)

func TestFromEnv_MockModeDefaultsOrigins(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("PLUGIN_PROXY_MODE", "mock")
	// Explicitly clear origin envs so we test the default behavior.
	t.Setenv("PLUGIN_PROXY_ORIGIN", "")
	t.Setenv("KUBE_API_PROXY_ORIGIN", "")
	t.Setenv("CONSOLE_ORIGIN", "")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.PluginProxyOrigin != "http://plugin-proxy.fundament.localhost:8080" {
		t.Errorf("PluginProxyOrigin = %q", cfg.PluginProxyOrigin)
	}
	if cfg.KubeAPIProxyOrigin != "http://kube-api-proxy.fundament.localhost:8080" {
		t.Errorf("KubeAPIProxyOrigin = %q", cfg.KubeAPIProxyOrigin)
	}
	if cfg.ConsoleOrigin != "http://console.fundament.localhost:8080" {
		t.Errorf("ConsoleOrigin = %q", cfg.ConsoleOrigin)
	}
	if cfg.JWTSecret != "test-secret" {
		t.Errorf("JWTSecret = %q", cfg.JWTSecret)
	}
}

func TestFromEnv_MockModePreservesOrigins(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("PLUGIN_PROXY_MODE", "mock")
	t.Setenv("PLUGIN_PROXY_ORIGIN", "https://pp.example")
	t.Setenv("KUBE_API_PROXY_ORIGIN", "https://kap.example")
	t.Setenv("CONSOLE_ORIGIN", "https://console.example")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.PluginProxyOrigin != "https://pp.example" {
		t.Errorf("PluginProxyOrigin = %q", cfg.PluginProxyOrigin)
	}
	if cfg.KubeAPIProxyOrigin != "https://kap.example" {
		t.Errorf("KubeAPIProxyOrigin = %q", cfg.KubeAPIProxyOrigin)
	}
	if cfg.ConsoleOrigin != "https://console.example" {
		t.Errorf("ConsoleOrigin = %q", cfg.ConsoleOrigin)
	}
}

func TestFromEnv_RealModeRequiresAllOrigins(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T)
	}{
		{
			name: "missing plugin proxy origin",
			setup: func(t *testing.T) {
				t.Setenv("PLUGIN_PROXY_ORIGIN", "")
				t.Setenv("KUBE_API_PROXY_ORIGIN", "https://kap.example")
				t.Setenv("CONSOLE_ORIGIN", "https://console.example")
			},
		},
		{
			name: "missing kube-api proxy origin",
			setup: func(t *testing.T) {
				t.Setenv("PLUGIN_PROXY_ORIGIN", "https://pp.example")
				t.Setenv("KUBE_API_PROXY_ORIGIN", "")
				t.Setenv("CONSOLE_ORIGIN", "https://console.example")
			},
		},
		{
			name: "missing console origin",
			setup: func(t *testing.T) {
				t.Setenv("PLUGIN_PROXY_ORIGIN", "https://pp.example")
				t.Setenv("KUBE_API_PROXY_ORIGIN", "https://kap.example")
				t.Setenv("CONSOLE_ORIGIN", "")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("JWT_SECRET", "test-secret")
			t.Setenv("PLUGIN_PROXY_MODE", "real")
			tc.setup(t)

			_, err := FromEnv()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "real mode") {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestFromEnv_RealModeWithAllOriginsSucceeds(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("PLUGIN_PROXY_MODE", "real")
	t.Setenv("PLUGIN_PROXY_ORIGIN", "https://pp.example")
	t.Setenv("KUBE_API_PROXY_ORIGIN", "https://kap.example")
	t.Setenv("CONSOLE_ORIGIN", "https://console.example")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.Mode != "real" {
		t.Errorf("Mode = %q", cfg.Mode)
	}
}

func TestFromEnv_UnknownModeErrors(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("PLUGIN_PROXY_MODE", "weird")

	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "weird") {
		t.Errorf("error should mention unknown mode value: %v", err)
	}
}

func TestFromEnv_MissingJWTSecretErrors(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	t.Setenv("PLUGIN_PROXY_MODE", "mock")

	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
