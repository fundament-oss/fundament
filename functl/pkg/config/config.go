// Package config provides configuration and credentials management for the Fundament CLI.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

// Config holds the Fundament CLI configuration.
type Config struct {
	APIEndpoint  string `yaml:"api_endpoint"`
	AuthnURL     string `yaml:"authn_url"`
	Output       string `yaml:"output"`
	Organization string `yaml:"organization,omitempty"`
}

// Credentials holds the API key for authentication.
type Credentials struct {
	APIKey string `yaml:"api_key"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		APIEndpoint: "http://organization.fundament.localhost:10080",
		AuthnURL:    "http://authn.fundament.localhost:10080",
		Output:      "table",
	}
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
	return os.MkdirAll(dir, 0o700)
}

// LoadConfig loads the configuration from the config file.
// If the file doesn't exist, it returns the default configuration.
func LoadConfig() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
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

	data, err := os.ReadFile(path)
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

	data, err := yaml.Marshal(creds)
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
