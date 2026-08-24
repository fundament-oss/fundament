package main

import (
	"fmt"
	"os"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

func main() {
	plugin, err := NewOpenFSCPlugin()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create plugin: %v\n", err)
		os.Exit(1)
	}

	pluginruntime.Run(plugin)
}
