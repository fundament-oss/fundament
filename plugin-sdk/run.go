package pluginsdk

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

	"github.com/fundament-oss/fundament/plugin-sdk/health"
	"github.com/fundament-oss/fundament/plugin-sdk/metadata"
	"github.com/fundament-oss/fundament/plugin-sdk/metadata/proto/gen/v1/pluginmetadatav1connect"
	"github.com/fundament-oss/fundament/plugin-sdk/observability"
)

// Run is the main entry point for plugin binaries. It handles all boilerplate:
// environment configuration, structured logging, HTTP server with health probes
// and metadata API, signal handling, and graceful shutdown.
//
// The plugin's Start method is called after setup completes. It should block
// until ctx is cancelled, then return. After Start returns, Shutdown is called
// with a deadline context.
func Run(plugin Plugin, opts ...RunOption) {
	cfg := defaultRunConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	var envCfg Config
	if err := env.Parse(&envCfg); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse config: %v\n", err)
		os.Exit(1)
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

	metadataServer := metadata.NewServer(
		func() metadata.Status {
			s := h.CurrentStatus()
			return metadata.Status{Phase: string(s.Phase), Message: s.Message}
		},
		func() metadata.Definition {
			return toMetadataDefinition(plugin.Definition())
		},
	)
	path, handler := pluginmetadatav1connect.NewPluginMetadataServiceHandler(metadataServer)
	mux.Handle(path, handler)

	if cp, ok := plugin.(ConsoleProvider); ok {
		mux.Handle("/console/", http.StripPrefix("/console/", http.FileServer(cp.ConsoleAssets())))
	}

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.metadataPort),
		Handler: mux,
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

	if err := g.Wait(); err != nil {
		logger.Error("plugin error", "error", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.shutdownTimeout)
	defer shutdownCancel()

	if err := plugin.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "error", err)
	}
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

func toMetadataDefinition(def PluginDefinition) metadata.Definition {
	orgMenu := make([]metadata.MenuEntryDef, len(def.Menu.Organization))
	for i, entry := range def.Menu.Organization {
		orgMenu[i] = metadata.MenuEntryDef{
			CRD:    entry.CRD,
			List:   entry.List,
			Detail: entry.Detail,
			Create: entry.Create,
			Icon:   entry.Icon,
		}
	}

	projectMenu := make([]metadata.MenuEntryDef, len(def.Menu.Project))
	for i, entry := range def.Menu.Project {
		projectMenu[i] = metadata.MenuEntryDef{
			CRD:    entry.CRD,
			List:   entry.List,
			Detail: entry.Detail,
			Create: entry.Create,
			Icon:   entry.Icon,
		}
	}

	rbacRules := make([]metadata.PolicyRuleDef, len(def.Permissions.RBAC))
	for i, rule := range def.Permissions.RBAC {
		rbacRules[i] = metadata.PolicyRuleDef{
			APIGroups: rule.APIGroups,
			Resources: rule.Resources,
			Verbs:     rule.Verbs,
		}
	}

	customComponents := make(map[string]metadata.ComponentMappingDef, len(def.CustomComponents))
	for k, v := range def.CustomComponents {
		customComponents[k] = metadata.ComponentMappingDef{
			List:   v.List,
			Detail: v.Detail,
		}
	}

	uiHints := make(map[string]metadata.UIHintDef, len(def.UIHints))
	for k, v := range def.UIHints {
		formGroups := make([]metadata.FormGroupDef, len(v.FormGroups))
		for i, fg := range v.FormGroups {
			formGroups[i] = metadata.FormGroupDef{
				Name:   fg.Name,
				Fields: fg.Fields,
			}
		}

		statusValues := make(map[string]metadata.StatusValueDef, len(v.StatusMapping.Values))
		for sk, sv := range v.StatusMapping.Values {
			statusValues[sk] = metadata.StatusValueDef{
				Badge: sv.Badge,
				Label: sv.Label,
			}
		}

		uiHints[k] = metadata.UIHintDef{
			FormGroups: formGroups,
			StatusMapping: metadata.StatusMappingDef{
				JSONPath: v.StatusMapping.JSONPath,
				Values:   statusValues,
			},
		}
	}

	return metadata.Definition{
		Name:        def.Metadata.Name,
		DisplayName: def.Metadata.DisplayName,
		Version:     def.Metadata.Version,
		Description: def.Metadata.Description,
		Author:      def.Metadata.Author,
		License:     def.Metadata.License,
		Icon:        def.Metadata.Icon,
		URLs: metadata.URLsDef{
			Homepage:      def.Metadata.URLs.Homepage,
			Repository:    def.Metadata.URLs.Repository,
			Documentation: def.Metadata.URLs.Documentation,
		},
		Tags: def.Metadata.Tags,
		Permissions: metadata.PermissionsDef{
			Capabilities: def.Permissions.Capabilities,
			RBAC:         rbacRules,
		},
		Menu: metadata.Menu{
			Organization: orgMenu,
			Project:      projectMenu,
		},
		CustomComponents: customComponents,
		UIHints:          uiHints,
		CRDs:             def.CRDs,
	}
}
