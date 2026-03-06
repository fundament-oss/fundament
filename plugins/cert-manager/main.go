package main

import (
	"fmt"
	"os"

	pluginsdk "github.com/fundament-oss/fundament/plugin-sdk"
)

func main() {
	def, err := pluginsdk.LoadDefinition("definition.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load definition: %v\n", err)
		os.Exit(1)
	}
	pluginsdk.Run(NewCertManagerPlugin(def))
}
