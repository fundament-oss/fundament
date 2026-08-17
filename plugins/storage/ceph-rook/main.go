package main

import (
	"log"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

func main() {
	p, err := NewPlugin()
	if err != nil {
		log.Fatal(err)
	}
	pluginruntime.Run(p)
}
