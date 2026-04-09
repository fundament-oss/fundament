// Package config handles parsing of .gomod-age.json configuration files.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config represents the .gomod-age.json configuration file.
type Config struct {
	Age      string       `json:"age"`
	Indirect bool         `json:"indirect"`
	Ignore   []string     `json:"ignore"`
	Allow    []AllowEntry `json:"allow"`

	// Parsed minimum age duration (populated by Resolve).
	MinAge time.Duration `json:"-"`
}

// AllowEntry represents an explicitly allowed module@version pair.
type AllowEntry struct {
	Module  string `json:"module"`
	Version string `json:"version"`
	Reason  string `json:"reason"`
}

// Load reads and parses a config file. Returns an empty config if the file
// does not exist.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

// Resolve applies defaults and parses the age string into a duration.
// Call this after merging CLI flags into the config.
func (c *Config) Resolve() error {
	if c.Age == "" {
		c.Age = "7d"
	}
	d, err := ParseDuration(c.Age)
	if err != nil {
		return fmt.Errorf("invalid age %q: %w", c.Age, err)
	}
	c.MinAge = d
	return nil
}

// IsAllowed returns true if the given module@version is explicitly allowed.
func (c *Config) IsAllowed(module, version string) (string, bool) {
	for _, a := range c.Allow {
		if a.Module == module && a.Version == version {
			return a.Reason, true
		}
	}
	return "", false
}

// IsIgnored returns true if the module path matches any ignore pattern.
func (c *Config) IsIgnored(module string) bool {
	for _, pattern := range c.Ignore {
		if matchGlob(pattern, module) {
			return true
		}
	}
	return false
}

// ParseDuration parses a duration string that supports "d" suffix for days
// in addition to Go's standard time.ParseDuration formats.
func ParseDuration(s string) (time.Duration, error) {
	if numStr, ok := strings.CutSuffix(s, "d"); ok {
		days, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid day duration: %w", err)
		}
		return time.Duration(days * float64(24*time.Hour)), nil
	}
	return time.ParseDuration(s)
}

// matchGlob matches a module path against a GOPRIVATE-style glob pattern.
// Supports trailing /* for prefix matching.
func matchGlob(pattern, module string) bool {
	if prefix, ok := strings.CutSuffix(pattern, "/*"); ok {
		return module == prefix || strings.HasPrefix(module, prefix+"/")
	}
	if prefix, ok := strings.CutSuffix(pattern, "*"); ok {
		return strings.HasPrefix(module, prefix)
	}
	return pattern == module
}
