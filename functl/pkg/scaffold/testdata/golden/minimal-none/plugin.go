package main

import (
	"context"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

// DemoPlugin implements the demo Fundament plugin.
type DemoPlugin struct{}

// NewDemoPlugin creates a new DemoPlugin.
func NewDemoPlugin() *DemoPlugin {
	return &DemoPlugin{}
}

// Start runs the plugin's business logic and blocks until ctx is cancelled.
//
// Start must be idempotent: the container is restarted on upgrades, node
// evictions and crashes, so this runs again from scratch every time. Check what
// already exists before creating it.
//
// Wrap failures with pluginerrors.NewTransient (the platform retries; the plugin
// reports "degraded") or pluginerrors.NewPermanent (no retry; "failed").
func (p *DemoPlugin) Start(ctx context.Context, host pluginruntime.Host) error {
	// TODO: do the work that makes this plugin useful.

	host.ReportReady()
	host.ReportStatus(pluginruntime.PluginStatus{
		Phase:   pluginruntime.PhaseRunning,
		Message: "demo is running",
	})

	<-ctx.Done()
	return nil
}

// Shutdown performs graceful cleanup. The context carries a deadline (30s by
// default). It must not uninstall anything: shutdown happens on every restart.
func (p *DemoPlugin) Shutdown(_ context.Context) error {
	return nil
}
