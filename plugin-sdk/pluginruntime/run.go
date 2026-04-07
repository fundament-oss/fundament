package pluginruntime

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
	"go.opentelemetry.io/otel"
	"golang.org/x/sync/errgroup"

	pluginerrors "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/errors"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/health"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/metadata/proto/gen/v1/pluginmetadatav1connect"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/observability"
)

// Run is the main entry point for plugin binaries. It handles all boilerplate:
// environment configuration, structured logging, HTTP server with health probes
// and metadata API, signal handling, and graceful shutdown.
//
// The plugin's Start method is called after setup completes. It should block
// until ctx is cancelled, then return. After Start returns, Shutdown is called
// with a deadline context.
func Run(plugin Plugin, opts ...RunOption) {
	if err := run(plugin, opts...); err != nil {
		fmt.Fprintf(os.Stderr, "plugin error: %v\n", err)
		os.Exit(1)
	}
}

func run(plugin Plugin, opts ...RunOption) error {
	cfg := defaultRunConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	var envCfg Config
	if err := env.Parse(&envCfg); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	logHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: envCfg.LogLevel})
	logger := slog.New(logHandler)

	telemetry := observability.NewTelemetryService(logger, otel.GetTracerProvider(), otel.GetMeterProvider())

	h := newHost(logger, telemetry)

	sigCtx, sigCancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer sigCancel()

	mux := http.NewServeMux()

	mux.Handle("GET /healthz", health.LivenessHandler())
	mux.Handle("GET /readyz", health.ReadinessHandler(h))

	uninstallFn := func(ctx context.Context) error {
		installer, ok := plugin.(Installer)
		if !ok {
			return nil
		}
		h.ReportStatus(PluginStatus{Phase: PhaseUninstalling, Message: "uninstalling"})
		if err := installer.Uninstall(ctx, h); err != nil {
			return err
		}
		sigCancel()
		return nil
	}

	handler := NewMetadataHandler(
		func() PluginStatus { return h.CurrentStatus() },
		func() PluginDefinition { return plugin.Definition() },
		uninstallFn,
	)
	path, rpcHandler := pluginmetadatav1connect.NewPluginMetadataServiceHandler(handler)
	mux.Handle(path, rpcHandler)

	if cp, ok := plugin.(ConsoleProvider); ok {
		mux.Handle("/console/", http.StripPrefix("/console/", http.FileServer(cp.ConsoleAssets())))
	}

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.metadataPort),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	g, ctx := errgroup.WithContext(sigCtx)

	g.Go(func() error {
		logger.Info("starting HTTP server", "port", cfg.metadataPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.shutdownTimeout)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	})

	g.Go(func() error {
		return plugin.Start(ctx, h)
	})

	if reconciler, ok := plugin.(Reconciler); ok {
		g.Go(func() error {
			return runReconcileLoop(ctx, h, reconciler, envCfg.ReconcileInterval)
		})
	}

	err := g.Wait()
	if err != nil {
		switch {
		case pluginerrors.IsPermanent(err):
			h.ReportStatus(PluginStatus{Phase: PhaseFailed, Message: err.Error()})
		case pluginerrors.IsTransient(err):
			h.ReportStatus(PluginStatus{Phase: PhaseDegraded, Message: err.Error()})
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.shutdownTimeout)
	defer shutdownCancel()

	if err := plugin.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "error", err)
	}

	if err != nil {
		return fmt.Errorf("plugin runtime: %w", err)
	}
	return nil
}

func runReconcileLoop(ctx context.Context, h *host, reconciler Reconciler, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := reconciler.Reconcile(ctx, h); err != nil {
				h.logger.Error("reconcile error", "error", err)
			}
		}
	}
}
