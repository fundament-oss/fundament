package pluginruntime

import (
	"context"
	"net/http"
)

// Plugin is the main interface that plugin developers implement.
type Plugin interface {
	// Definition returns the static metadata for this plugin.
	Definition() PluginDefinition

	// Start runs the plugin's business logic. It should block until ctx is cancelled.
	// The plugin should call host.ReportReady() once it is ready to serve traffic.
	Start(ctx context.Context, host Host) error

	// Shutdown performs graceful cleanup. The context has a deadline set by
	// WithShutdownTimeout (default 30s).
	Shutdown(ctx context.Context) error
}

// Reconciler is an optional interface for plugins that need periodic reconciliation.
// If a Plugin also implements Reconciler, the Run harness calls Reconcile at the
// configured interval (RECONCILE_INTERVAL env var, default 5m).
type Reconciler interface {
	Reconcile(ctx context.Context, host Host) error
}

// ConsoleProvider is an optional interface for plugins that serve console UI assets.
// When implemented, the Run harness mounts the assets at /console/.
type ConsoleProvider interface {
	ConsoleAssets() http.FileSystem
}

// Installer is an optional interface for plugins with structured install/uninstall/upgrade lifecycle.
type Installer interface {
	Install(ctx context.Context, host Host) error
	Uninstall(ctx context.Context, host Host) error
	Upgrade(ctx context.Context, host Host) error
}
