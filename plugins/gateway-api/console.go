package main

import (
	"embed"
	"net/http"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/console"
)

//go:embed console/*
var consoleFiles embed.FS

func (p *GatewayAPIPlugin) ConsoleAssets() http.FileSystem {
	return console.NewFileSystem(consoleFiles, "console")
}
