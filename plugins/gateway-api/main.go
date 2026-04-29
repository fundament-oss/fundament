package main

import (
	"fmt"
	"os"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

func main() {
	def, err := pluginruntime.LoadDefinition("definition.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load definition: %v\n", err)
		os.Exit(1)
	}

	plugin, err := NewGatewayAPIPlugin(&def)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create plugin: %v\n", err)
		os.Exit(1)
	}

	pluginruntime.Run(plugin)
}
