package main

import (
	"embed"
	"net/http"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/console"
)

//go:embed console/*
var consoleFiles embed.FS

// ConsoleAssets makes *Plugin a pluginruntime.ConsoleProvider, which is what
// registers the /console/ route in pluginruntime.Run. Without it the console
// iframe 404s and the files under console/ are never served.
//
// RequireHTML: this plugin's console is hand-written and checked in (no build
// step), so a missing HTML file means the embed pattern broke rather than an
// unbuilt UI — fail at startup instead of serving a blank iframe.
func (p *Plugin) ConsoleAssets() http.FileSystem {
	return console.NewFileSystem(consoleFiles, "console", console.RequireHTML())
}
