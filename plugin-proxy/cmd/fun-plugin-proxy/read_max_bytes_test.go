package main

import (
	"log/slog"
	"net/http"
	"testing"

	"github.com/fundament-oss/fundament/common/connectlimit/connectlimittest"
	"github.com/fundament-oss/fundament/plugin-proxy/pkg/proto/gen/plugin_proxy/v1/pluginproxyv1connect"
	"github.com/fundament-oss/fundament/plugin-proxy/pkg/service"
)

func TestRejectsGzipBombBeforeAuth(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	mux := http.NewServeMux()
	registerRPCHandlers(mux, service.New(logger, nil), logger)

	connectlimittest.AssertRejectsGzipBomb(t, mux, pluginproxyv1connect.PluginInstallationServiceGetInstallationManifestProcedure)
}
