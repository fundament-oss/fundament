package config

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/discovery"
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
	t.Setenv(EnvURL, "https://fundament.env.example")
	t.Setenv(EnvAPIEndpoint, "https://api.env.example")
	t.Setenv(EnvAuthnURL, "https://authn.env.example")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "https://fundament.env.example", cfg.URL, "URL should come from the env override")
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

// discoveryServer serves a discovery document naming itself as issuer.
func discoveryServer(t *testing.T, fields string) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != discovery.Path {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"issuer": %q%s}`, server.URL, fields)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestResolve_Discovers(t *testing.T) {
	server := discoveryServer(t, `,
		"organization_api": "https://organization.example.com",
		"authn_api": "https://authn.example.com",
		"marketplace_registry_api": "https://registry.example.com"`)

	cfg := &Config{URL: server.URL}
	require.NoError(t, cfg.Resolve(t.Context()))

	assert.Equal(t, "https://organization.example.com", cfg.APIEndpoint)
	assert.Equal(t, "https://authn.example.com", cfg.AuthnURL)
	assert.Equal(t, "https://registry.example.com", cfg.RegistryURL)
}

func TestResolve_ExplicitEndpointWins(t *testing.T) {
	server := discoveryServer(t, `,
		"organization_api": "https://organization.example.com",
		"authn_api": "https://authn.example.com"`)

	cfg := &Config{URL: server.URL, AuthnURL: "https://authn.override.example"}
	require.NoError(t, cfg.Resolve(t.Context()))

	assert.Equal(t, "https://organization.example.com", cfg.APIEndpoint)
	assert.Equal(t, "https://authn.override.example", cfg.AuthnURL)
}

func TestResolve_ExplicitEndpointsNeedNoDiscovery(t *testing.T) {
	cfg := &Config{
		URL:         "https://unreachable.invalid",
		APIEndpoint: "https://organization.example.com",
		AuthnURL:    "https://authn.example.com",
	}
	require.NoError(t, cfg.Resolve(t.Context()))
	assert.Empty(t, cfg.RegistryURL)
}

// One explicit endpoint without a URL must not fall back to the default
// installation: the API key would go to an installation it is not for.
func TestResolve_PartialEndpointsWithoutURL(t *testing.T) {
	cfg := &Config{APIEndpoint: "https://organization.example.com"}
	require.ErrorContains(t, cfg.Resolve(t.Context()), "no installation URL")
}

func TestResolve_IncompleteDocument(t *testing.T) {
	server := discoveryServer(t, `, "organization_api": "https://organization.example.com"`)

	cfg := &Config{URL: server.URL}
	require.ErrorContains(t, cfg.Resolve(t.Context()), "names no organization or authn API")
}

func TestResolveRegistry(t *testing.T) {
	server := discoveryServer(t, `, "marketplace_registry_api": "https://registry.example.com"`)

	cfg := &Config{
		URL:         server.URL,
		APIEndpoint: "https://organization.example.com",
		AuthnURL:    "https://authn.example.com",
	}
	require.NoError(t, cfg.ResolveRegistry(t.Context()))
	assert.Equal(t, "https://registry.example.com", cfg.RegistryURL)
}

// Until the default installation serves a discovery document, an unconfigured
// functl keeps its former endpoints there.
func TestResolve_DefaultInstallationWithoutDocument(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	defaultURL = server.URL
	t.Cleanup(func() { defaultURL = DefaultURL })

	cfg := &Config{}
	require.NoError(t, cfg.Resolve(t.Context()))
	require.NoError(t, (&Config{}).ResolveRegistry(t.Context()))
	assert.Equal(t, int32(1), requests.Load(), "the legacy fallback is fetched once, too")

	assert.Equal(t, legacyDefaultAPIEndpoint, cfg.APIEndpoint)
	assert.Equal(t, legacyDefaultAuthnURL, cfg.AuthnURL)
}

func TestResolve_ConfiguredInstallationWithoutDocument(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)

	cfg := &Config{URL: server.URL}
	require.ErrorIs(t, cfg.Resolve(t.Context()), discovery.ErrNotFound)
}

func TestResolve_DefaultInstallationFailingIsNotPaperedOver(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)
	defaultURL = server.URL
	t.Cleanup(func() { defaultURL = DefaultURL })

	cfg := &Config{}
	require.ErrorContains(t, cfg.Resolve(t.Context()), "502")
	assert.Empty(t, cfg.APIEndpoint, "no fallback to the legacy endpoints")
}

func TestResolve_FetchesEachInstallationOnce(t *testing.T) {
	var requests atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"issuer": %q, "organization_api": "https://organization.example.com", "authn_api": "https://authn.example.com", "marketplace_registry_api": "https://registry.example.com"}`, server.URL)
	}))
	t.Cleanup(server.Close)

	first := &Config{URL: server.URL}
	require.NoError(t, first.Resolve(t.Context()))
	second := &Config{URL: server.URL, APIEndpoint: "https://organization.example.com", AuthnURL: "https://authn.example.com"}
	require.NoError(t, second.ResolveRegistry(t.Context()))

	assert.Equal(t, "https://registry.example.com", second.RegistryURL)
	assert.Equal(t, int32(1), requests.Load())
}

func TestLoadConfig_DropsEndpointsSavedAsOldDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FUNCTL_CONFIG_DIR", dir)
	t.Setenv(EnvURL, "")
	t.Setenv(EnvAPIEndpoint, "")
	t.Setenv(EnvAuthnURL, "")
	content := "url: https://fundament.file.example\napi_endpoint: " + legacyDefaultAPIEndpoint + "\nauthn_url: " + legacyDefaultAuthnURL + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0o600))

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "https://fundament.file.example", cfg.URL)
	assert.Empty(t, cfg.APIEndpoint, "the old default must not pin the PoC")
	assert.Empty(t, cfg.AuthnURL)
}

func TestLoadConfig_KeepsEndpointsSetOnPurpose(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FUNCTL_CONFIG_DIR", dir)
	t.Setenv(EnvURL, "")
	t.Setenv(EnvAPIEndpoint, "")
	t.Setenv(EnvAuthnURL, "")
	content := "api_endpoint: https://organization.other.example\nauthn_url: " + legacyDefaultAuthnURL + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0o600))

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "https://organization.other.example", cfg.APIEndpoint)
	assert.Equal(t, legacyDefaultAuthnURL, cfg.AuthnURL)
}

func TestLoadFileConfig_IgnoresEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FUNCTL_CONFIG_DIR", dir)
	t.Setenv(EnvURL, "https://fundament.env.example")
	t.Setenv(EnvAPIEndpoint, "https://api.env.example")
	content := "url: https://fundament.file.example\norganization: org-1\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0o600))

	cfg, err := LoadFileConfig()
	require.NoError(t, err)
	assert.Equal(t, "https://fundament.file.example", cfg.URL)
	assert.Empty(t, cfg.APIEndpoint)
	assert.Equal(t, "org-1", cfg.Organization)

	cfg.Organization = "org-2"
	require.NoError(t, SaveConfig(cfg))

	saved, err := os.ReadFile(filepath.Join(dir, "config.yaml")) //nolint:gosec // a file in the test's own temp dir
	require.NoError(t, err)
	assert.NotContains(t, string(saved), "env.example", "environment values must not reach the file")
}

func TestResolveRegistry_ExplicitEndpointsNameRegistryURL(t *testing.T) {
	cfg := &Config{APIEndpoint: "https://organization.example.com", AuthnURL: "https://authn.example.com"}
	require.ErrorContains(t, cfg.ResolveRegistry(t.Context()), "set registry_url")
}
