package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

func TestPluginImplementsInterfaces(t *testing.T) {
	t.Parallel()
	plugin := NewExternalDNSPlugin()

	// Verify all expected interfaces are implemented.
	var _ pluginruntime.Plugin = plugin
	var _ pluginruntime.Installer = plugin
	var _ pluginruntime.Reconciler = plugin
	var _ pluginruntime.ConsoleProvider = plugin

	assert.NotNil(t, plugin)
}
