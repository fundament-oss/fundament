package main

import (
	"embed"
	"net/http"

	"github.com/fundament-oss/fundament/plugin-sdk/console"
)

//go:embed console/*
var consoleFiles embed.FS

func (p *CertManagerPlugin) ConsoleAssets() http.FileSystem {
	return console.NewFileSystem(consoleFiles, "console")
}
