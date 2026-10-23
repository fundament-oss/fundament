package plugindev

import (
	"fmt"
	"maps"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

// ResolveConfig merges the config file (if any) with set, set winning, and
// validates the result against the definition's configSchema.
func ResolveConfig(definition []byte, file string, set map[string]string) (map[string]string, error) {
	config := map[string]string{}
	if file != "" {
		raw, err := os.ReadFile(file) //nolint:gosec // path comes from the developer's own CLI flag
		if err != nil {
			return nil, fmt.Errorf("read config file: %w", err)
		}
		if err := yaml.Unmarshal(raw, &config); err != nil {
			return nil, fmt.Errorf("parse config file %s (want a map of KEY: value): %w", file, err)
		}
	}
	maps.Copy(config, set)

	var def pluginruntime.PluginDefinition
	if err := yaml.Unmarshal(definition, &def); err != nil {
		return nil, fmt.Errorf("parse definition.yaml: %w", err)
	}
	if err := missingRequired(config, def.Spec.ConfigSchema); err != nil {
		return nil, err
	}
	if err := pluginruntime.ValidateConfig(config, def.Spec.ConfigSchema); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return config, nil
}

// missingRequired lists every required key without a value, so one run reports them all.
func missingRequired(config map[string]string, schema []pluginruntime.ConfigSchemaEntry) error {
	var b strings.Builder
	for _, entry := range schema {
		if !entry.Required || strings.TrimSpace(config[entry.Name]) != "" {
			continue
		}
		fmt.Fprintf(&b, "\n  %s", entry.Name)
		if entry.DisplayName != "" {
			fmt.Fprintf(&b, " (%s)", entry.DisplayName)
		}
		if entry.Description != "" {
			fmt.Fprintf(&b, ": %s", entry.Description)
		}
	}
	if b.Len() == 0 {
		return nil
	}
	return fmt.Errorf("definition.yaml requires config values that are not set:%s\nPass them with --set KEY=VALUE or a --config file", b.String())
}
