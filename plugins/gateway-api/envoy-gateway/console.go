package main

import (
	"embed"
	"net/http"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/console"
)

//go:embed console/*
var consoleFiles embed.FS

// RequireHTML: console/ holds the Vite build output (gitignored) — a binary
// built without it would serve a blank iframe instead of failing.
func (p *EnvoyGatewayPlugin) ConsoleAssets() http.FileSystem {
	return console.NewFileSystem(consoleFiles, "console", console.RequireHTML())
}
