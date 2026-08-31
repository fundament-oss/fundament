package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// absDir returns a platform-appropriate absolute path to use as a config dir.
// Literal POSIX paths such as "/tmp/functl-config" are not absolute on Windows,
// where filepath.IsAbs also requires a volume name.
func absDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.True(t, filepath.IsAbs(dir), "t.TempDir() should return an absolute path")
	return dir
}

// wantDefaultConfigDir returns the directory ConfigDir documents as its
// fallback once FUNCTL_CONFIG_DIR and XDG_CONFIG_HOME are out of the picture:
// %APPDATA%/fundament on Windows, ~/.config/fundament elsewhere.
func wantDefaultConfigDir(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "fundament")
		}
	}
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	return filepath.Join(home, ".config", "fundament")
}

func TestConfigDir_FunclConfigDir_Absolute(t *testing.T) {
	want := absDir(t)
	t.Setenv("FUNCTL_CONFIG_DIR", want)
	t.Setenv("XDG_CONFIG_HOME", "")

	dir, err := ConfigDir()
	require.NoError(t, err)
	assert.Equal(t, want, dir)
}

func TestConfigDir_FunclConfigDir_Relative(t *testing.T) {
	t.Setenv("FUNCTL_CONFIG_DIR", filepath.Join("relative", "path"))

	_, err := ConfigDir()
	require.Error(t, err, "a relative FUNCTL_CONFIG_DIR should be rejected")
}

func TestConfigDir_FunclConfigDir_Precedence(t *testing.T) {
	want := absDir(t)
	t.Setenv("FUNCTL_CONFIG_DIR", want)
	t.Setenv("XDG_CONFIG_HOME", absDir(t))

	dir, err := ConfigDir()
	require.NoError(t, err)
	assert.Equal(t, want, dir)
}

func TestConfigDir_XDGConfigHome_Absolute(t *testing.T) {
	xdg := absDir(t)
	t.Setenv("FUNCTL_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", xdg)

	dir, err := ConfigDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(xdg, "fundament"), dir)
}

func TestConfigDir_XDGConfigHome_Relative(t *testing.T) {
	t.Setenv("FUNCTL_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join("relative", "config"))

	dir, err := ConfigDir()
	require.NoError(t, err)
	assert.Equal(t, wantDefaultConfigDir(t), dir, "a relative XDG_CONFIG_HOME should fall through to the default")
}

// TestConfigDir_AppData covers the Windows-only %APPDATA% branch, which sits
// between XDG_CONFIG_HOME and the ~/.config fallback.
func TestConfigDir_AppData(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the APPDATA branch only applies on Windows")
	}
	appData := absDir(t)
	t.Setenv("FUNCTL_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("APPDATA", appData)

	dir, err := ConfigDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(appData, "fundament"), dir)
}

// TestConfigDir_HomeFallback covers the final ~/.config fallback. On Windows
// that needs APPDATA cleared, because the branch above would otherwise win.
func TestConfigDir_HomeFallback(t *testing.T) {
	t.Setenv("FUNCTL_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", "")
	}

	dir, err := ConfigDir()
	require.NoError(t, err)

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".config", "fundament"), dir)
}

func TestConfigDir_Default(t *testing.T) {
	t.Setenv("FUNCTL_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	dir, err := ConfigDir()
	require.NoError(t, err)
	assert.Equal(t, wantDefaultConfigDir(t), dir)
}

func TestLoadConfig_EnvOverrides_NoConfigFile(t *testing.T) {
	t.Setenv("FUNCTL_CONFIG_DIR", t.TempDir())
	t.Setenv(EnvAPIEndpoint, "https://api.env.example")
	t.Setenv(EnvAuthnURL, "https://authn.env.example")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "https://api.env.example", cfg.APIEndpoint, "APIEndpoint should come from the env override")
	assert.Equal(t, "https://authn.env.example", cfg.AuthnURL, "AuthnURL should come from the env override")
}

func TestLoadConfig_EnvOverrides_ConfigFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FUNCTL_CONFIG_DIR", dir)
	content := "api_endpoint: https://api.file.example\nauthn_url: https://authn.file.example\noutput: json\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0o600))

	// Only the API endpoint is overridden; an empty env var counts as unset.
	// Both vars are set explicitly because dev shells export them via mise.
	t.Setenv(EnvAPIEndpoint, "https://api.env.example")
	t.Setenv(EnvAuthnURL, "")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "https://api.env.example", cfg.APIEndpoint, "APIEndpoint should come from the env override")
	assert.Equal(t, "https://authn.file.example", cfg.AuthnURL, "AuthnURL should come from the config file")
	assert.Equal(t, "json", cfg.Output, "Output should come from the config file")
}
