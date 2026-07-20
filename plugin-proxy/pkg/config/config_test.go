package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromEnv_MockModeDefaultsOrigins(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("OPENFGA_API_URL", "http://openfga:8080")
	t.Setenv("OPENFGA_STORE_ID", "test-store")
	t.Setenv("PLUGIN_PROXY_MODE", "mock")
	t.Setenv("PLUGIN_SDK_DIR", t.TempDir())
	// Explicitly clear origin envs so we test the default behavior.
	t.Setenv("PLUGIN_PROXY_ORIGIN", "")
	t.Setenv("KUBE_API_PROXY_ORIGIN", "")
	t.Setenv("CONSOLE_ORIGIN", "")

	cfg, err := FromEnv()
	require.NoError(t, err)
	assert.Equal(t, "http://plugin-proxy.fundament.localhost:8080", cfg.PluginProxyOrigin)
	assert.Equal(t, "http://kube-api-proxy.fundament.localhost:8080", cfg.KubeAPIProxyOrigin)
	assert.Equal(t, "http://console.fundament.localhost:8080", cfg.ConsoleOrigin)
	assert.Equal(t, "test-secret", cfg.JWTSecret)
}

func TestFromEnv_MockModePreservesOrigins(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("OPENFGA_API_URL", "http://openfga:8080")
	t.Setenv("OPENFGA_STORE_ID", "test-store")
	t.Setenv("PLUGIN_PROXY_MODE", "mock")
	t.Setenv("PLUGIN_SDK_DIR", t.TempDir())
	t.Setenv("PLUGIN_PROXY_ORIGIN", "https://pp.example")
	t.Setenv("KUBE_API_PROXY_ORIGIN", "https://kap.example")
	t.Setenv("CONSOLE_ORIGIN", "https://console.example")

	cfg, err := FromEnv()
	require.NoError(t, err)
	assert.Equal(t, "https://pp.example", cfg.PluginProxyOrigin)
	assert.Equal(t, "https://kap.example", cfg.KubeAPIProxyOrigin)
	assert.Equal(t, "https://console.example", cfg.ConsoleOrigin)
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
			t.Setenv("OPENFGA_API_URL", "http://openfga:8080")
			t.Setenv("OPENFGA_STORE_ID", "test-store")
			t.Setenv("PLUGIN_PROXY_MODE", "real")
			tc.setup(t)

			_, err := FromEnv()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "real mode")
		})
	}
}

func TestFromEnv_RealModeWithAllOriginsSucceeds(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("OPENFGA_API_URL", "http://openfga:8080")
	t.Setenv("OPENFGA_STORE_ID", "test-store")
	t.Setenv("PLUGIN_PROXY_MODE", "real")
	t.Setenv("PLUGIN_PROXY_ORIGIN", "https://pp.example")
	t.Setenv("KUBE_API_PROXY_ORIGIN", "https://kap.example")
	t.Setenv("CONSOLE_ORIGIN", "https://console.example")

	cfg, err := FromEnv()
	require.NoError(t, err)
	assert.Equal(t, "real", cfg.Mode)
}

func TestFromEnv_UnknownModeErrors(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("OPENFGA_API_URL", "http://openfga:8080")
	t.Setenv("OPENFGA_STORE_ID", "test-store")
	t.Setenv("PLUGIN_PROXY_MODE", "weird")

	_, err := FromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "weird")
}

func TestFromEnv_MissingJWTSecretErrors(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	t.Setenv("OPENFGA_API_URL", "http://openfga:8080")
	t.Setenv("OPENFGA_STORE_ID", "test-store")
	t.Setenv("PLUGIN_PROXY_MODE", "mock")

	_, err := FromEnv()
	require.Error(t, err)
}
