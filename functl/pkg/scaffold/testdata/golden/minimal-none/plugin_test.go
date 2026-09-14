package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

func TestPluginImplementsInterfaces(t *testing.T) {
	t.Parallel()
	plugin := NewDemoPlugin()

	var _ pluginruntime.Plugin = plugin

	assert.NotNil(t, plugin)
}
