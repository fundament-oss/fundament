package main

import (
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

func main() {
	pluginruntime.Run(NewCertManagerPlugin())
}
