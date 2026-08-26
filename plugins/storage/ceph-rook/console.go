package main

import (
	"embed"
	"net/http"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/console"
)

//go:embed console/*
var consoleFiles embed.FS

// ConsoleAssets makes *Plugin a ConsoleProvider, which is what registers the
// /console/ route in pluginruntime.Run; without it the iframe 404s.
//
// RequireHTML: the console is hand-written and checked in, so a missing file
// means the embed pattern broke. Fail at startup, not with a blank iframe.
func (p *Plugin) ConsoleAssets() http.FileSystem {
	return console.NewFileSystem(consoleFiles, "console", console.RequireHTML())
}
