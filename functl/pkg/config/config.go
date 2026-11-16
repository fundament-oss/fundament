// Package config provides configuration and credentials management for the Fundament CLI.
package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/fundament-oss/fundament/common/discovery"
)

// Config holds the Fundament CLI configuration.
type Config struct {
	// Root URL of the installation. Its discovery document
	// (/.well-known/fundament.json) names the endpoints below, so those only
	// need setting to override it.
	URL         string `yaml:"url,omitempty"`
	APIEndpoint string `yaml:"api_endpoint,omitempty"`
	AuthnURL    string `yaml:"authn_url,omitempty"`
	// Base URL of marketplace-registry-api, the plugin publishing surface
	// (FUN-20). Empty after Resolve means publishing commands refuse to run.
	RegistryURL  string `yaml:"registry_url,omitempty"`
	Output       string `yaml:"output"`
	Organization string `yaml:"organization,omitempty"`
}

// Credentials holds the API key for authentication.
type Credentials struct {
	APIKey string `yaml:"api_key"`
}

// DefaultURL is the installation functl talks to when nothing is configured.
const DefaultURL = "https://fundament-poc.nl"

// The default installation's endpoints from before it served a discovery
// document. To be removed once it serves one:
// https://gitlab.com/digilab.overheid.nl/miscellaneous/issues/-/work_items/1480
const (
	legacyDefaultAPIEndpoint = "https://organization-api.fundament-poc.nl"
	legacyDefaultAuthnURL    = "https://authn.fundament-poc.nl"
)

// defaultURL is DefaultURL, replaceable in tests.
var defaultURL = DefaultURL

// documents holds the discovery documents fetched by this process, by
// installation URL, so one command fetches each at most once.
var documents struct {
	sync.Mutex
	byURL map[string]*discovery.Document
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Output: "table",
	}
}

// Environment variables that override the endpoint configuration.
const (
	EnvURL         = "FUNCTL_URL"
	EnvAPIEndpoint = "FUNCTL_API_ENDPOINT"
	EnvAuthnURL    = "FUNCTL_AUTHN_URL"
	EnvRegistryURL = "FUNCTL_REGISTRY_URL"
)

// dropLegacyDefaults forgets endpoints that older versions of functl wrote into
// the config file as their defaults (`functl org set` saved them). Left in
// place they would pin the PoC and make a configured url a no-op.
func dropLegacyDefaults(cfg *Config) {
	if cfg.APIEndpoint == legacyDefaultAPIEndpoint && cfg.AuthnURL == legacyDefaultAuthnURL {
		cfg.APIEndpoint = ""
		cfg.AuthnURL = ""
	}
}

// applyEnvOverrides overrides endpoint settings from the environment. They
// take precedence over the config file; the fundament repo sets EnvURL via
// mise so a checkout talks to the local dev installation.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv(EnvURL); v != "" {
		cfg.URL = v
	}
	if v := os.Getenv(EnvAPIEndpoint); v != "" {
		cfg.APIEndpoint = v
	}
	if v := os.Getenv(EnvAuthnURL); v != "" {
		cfg.AuthnURL = v
	}
	if v := os.Getenv(EnvRegistryURL); v != "" {
		cfg.RegistryURL = v
	}
}

// Resolve fills the API and authn endpoints that are not set explicitly from
// the installation's discovery document. It makes no request when both are set.
func (c *Config) Resolve(ctx context.Context) error {
	if c.APIEndpoint != "" && c.AuthnURL != "" {
		return nil
	}

	doc, baseURL, err := c.discover(ctx)
	if err != nil {
		return err
	}

	if c.APIEndpoint == "" {
		c.APIEndpoint = doc.OrganizationAPI
	}
	if c.AuthnURL == "" {
		c.AuthnURL = doc.AuthnAPI
	}
	if c.RegistryURL == "" {
		c.RegistryURL = doc.MarketplaceRegistryAPI
	}

	if c.APIEndpoint == "" || c.AuthnURL == "" {
		return fmt.Errorf("the discovery document of %s names no organization or authn API", baseURL)
	}
	return nil
}

// ResolveRegistry fills RegistryURL from the discovery document when it is not
// set explicitly. It stays empty when the installation has no registry.
func (c *Config) ResolveRegistry(ctx context.Context) error {
	if c.RegistryURL != "" {
		return nil
	}
	if c.URL == "" && (c.APIEndpoint != "" || c.AuthnURL != "") {
		return errors.New("no registry endpoint: set registry_url in the functl config or FUNCTL_REGISTRY_URL, or set url to discover it")
	}

	doc, _, err := c.discover(ctx)
	if err != nil {
		return err
	}
	c.RegistryURL = doc.MarketplaceRegistryAPI
	return nil
}

func (c *Config) discover(ctx context.Context) (*discovery.Document, string, error) {
	baseURL := c.URL
	if baseURL == "" {
		// An explicit endpoint points at some installation; the default URL
		// would send the API key to another one.
		if c.APIEndpoint != "" || c.AuthnURL != "" {
			return nil, "", errors.New("no installation URL: set url in the functl config or FUNCTL_URL, or set every endpoint explicitly")
		}
		baseURL = defaultURL
	}

	documents.Lock()
	defer documents.Unlock()
	if doc, ok := documents.byURL[baseURL]; ok {
		return doc, baseURL, nil
	}

	doc, err := discovery.Fetch(ctx, nil, baseURL)
	if err != nil {
		// Only a missing document means the default installation predates
		// discovery; a failing or invalid one must not be papered over.
		if c.URL != "" || !errors.Is(err, discovery.ErrNotFound) {
			return nil, "", fmt.Errorf("discovering the endpoints of %s: %w", baseURL, err)
		}
		doc = &discovery.Document{
			OrganizationAPI: legacyDefaultAPIEndpoint,
			AuthnAPI:        legacyDefaultAuthnURL,
		}
	}

	if documents.byURL == nil {
		documents.byURL = make(map[string]*discovery.Document)
	}
	documents.byURL[baseURL] = doc
	return doc, baseURL, nil
}

// ConfigDir returns the path to the configuration directory.
//
// Resolution order:
//  1. FUNCTL_CONFIG_DIR environment variable (explicit override)
//  2. XDG_CONFIG_HOME/fundament (XDG spec)
//  3. %APPDATA%/fundament (Windows default)
//  4. ~/.config/fundament (Linux/macOS CLI convention)
func ConfigDir() (string, error) {
	if v := os.Getenv("FUNCTL_CONFIG_DIR"); v != "" {
		if !filepath.IsAbs(v) {
			return "", fmt.Errorf("FUNCTL_CONFIG_DIR must be an absolute path, got: %s", v)
		}
		return v, nil
	}
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" && filepath.IsAbs(v) {
		return filepath.Join(v, "fundament"), nil
	}
	if runtime.GOOS == "windows" {
		if v := os.Getenv("APPDATA"); v != "" {
			return filepath.Join(v, "fundament"), nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".config", "fundament"), nil
}

// ConfigPath returns the path to the configuration file.
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// CredentialsPath returns the path to the credentials file.
func CredentialsPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials"), nil
}

// EnsureConfigDir creates the configuration directory if it doesn't exist.
func EnsureConfigDir() error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	return nil
}

// LoadConfig loads the configuration from the config file.
// If the file doesn't exist, it returns the default configuration.
// The FUNCTL_URL, FUNCTL_API_ENDPOINT, FUNCTL_AUTHN_URL and
// FUNCTL_REGISTRY_URL environment variables override the file. Endpoints left
// empty are filled by Resolve.
func LoadConfig() (*Config, error) {
	return loadConfig(true)
}

// LoadFileConfig loads the configuration as it is in the config file, without
// the environment overrides. Commands that change the file start from this, so
// that a value from the environment (such as the repo's FUNCTL_URL from mise)
// is not saved into it.
func LoadFileConfig() (*Config, error) {
	return loadConfig(false)
}

func loadConfig(withEnv bool) (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path) //nolint:gosec // path is functl's own config file
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg := DefaultConfig()
			if withEnv {
				applyEnvOverrides(cfg)
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := DefaultConfig()
	err = yaml.Unmarshal(data, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	dropLegacyDefaults(cfg)
	if withEnv {
		applyEnvOverrides(cfg)
	}

	return cfg, nil
}

// SaveConfig saves the configuration to the config file.
func SaveConfig(cfg *Config) error {
	if err := EnsureConfigDir(); err != nil {
		return err
	}

	path, err := ConfigPath()
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// LoadCredentials loads the credentials from the credentials file or environment.
func LoadCredentials() (*Credentials, error) {
	// Check environment variable first
	if apiKey := os.Getenv("FUNDAMENT_API_KEY"); apiKey != "" {
		return &Credentials{APIKey: apiKey}, nil
	}

	path, err := CredentialsPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path) //nolint:gosec // path is functl's own credentials file
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotAuthenticated
		}
		return nil, fmt.Errorf("failed to read credentials file: %w", err)
	}

	creds := &Credentials{}
	if err := yaml.Unmarshal(data, creds); err != nil {
		return nil, fmt.Errorf("failed to parse credentials file: %w", err)
	}

	if creds.APIKey == "" {
		return nil, ErrNotAuthenticated
	}

	return creds, nil
}

// SaveCredentials saves the credentials to the credentials file.
func SaveCredentials(creds *Credentials) error {
	if err := EnsureConfigDir(); err != nil {
		return err
	}

	path, err := CredentialsPath()
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(creds) //nolint:gosec // the credentials file stores the API key by design; written 0600
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	// Write with restricted permissions (owner read/write only)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	return nil
}

// DeleteCredentials removes the credentials file.
func DeleteCredentials() error {
	path, err := CredentialsPath()
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove credentials file: %w", err)
	}

	return nil
}

// ErrNotAuthenticated is returned when no API key is configured.
var ErrNotAuthenticated = errors.New("not authenticated: run 'functl auth login' to authenticate")
