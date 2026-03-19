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
	pluginruntime.Run(NewCertManagerPlugin(&def))
}
