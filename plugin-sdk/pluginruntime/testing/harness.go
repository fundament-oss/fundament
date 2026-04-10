// Package sdktesting provides test helpers for plugin developers.
package sdktesting

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/metadata/proto/gen/v1/pluginmetadatav1connect"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/observability"
)

// MockHost implements pluginruntime.Host for unit testing.
type MockHost struct {
	logger    *slog.Logger
	telemetry observability.TelemetryService
	ready     atomic.Bool
	status    pluginruntime.PluginStatus
	statusMu  sync.RWMutex

	// StatusHistory records all status updates for test assertions.
	StatusHistory []pluginruntime.PluginStatus
}

// NewMockHost creates a MockHost with a discard logger.
func NewMockHost() *MockHost {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return &MockHost{
		logger:    logger,
		telemetry: observability.NewTelemetryService(logger, nil, nil),
	}
}

// NewMockHostWithLogger creates a MockHost with the given logger.
func NewMockHostWithLogger(logger *slog.Logger) *MockHost {
	return &MockHost{
		logger:    logger,
		telemetry: observability.NewTelemetryService(logger, nil, nil),
	}
}

func (h *MockHost) Logger() *slog.Logger {
	return h.logger
}

func (h *MockHost) Telemetry() observability.TelemetryService {
	return h.telemetry
}

func (h *MockHost) ReportStatus(status pluginruntime.PluginStatus) {
	h.statusMu.Lock()
	defer h.statusMu.Unlock()
	h.status = status
	h.StatusHistory = append(h.StatusHistory, status)
}

func (h *MockHost) ReportReady() {
	h.ready.Store(true)
}

// IsReady reports whether ReportReady has been called.
func (h *MockHost) IsReady() bool {
	return h.ready.Load()
}

// CurrentStatus returns the current plugin status.
func (h *MockHost) CurrentStatus() pluginruntime.PluginStatus {
	h.statusMu.RLock()
	defer h.statusMu.RUnlock()
	return h.status
}

// InProcessPlugin runs a plugin in-process for integration testing.
// It starts the plugin with an HTTP server and returns a metadata client
// and a cancel function to stop the plugin.
type InProcessPlugin struct {
	Plugin         pluginruntime.Plugin
	MetadataClient pluginmetadatav1connect.PluginMetadataServiceClient
	BaseURL        string
	cancel         context.CancelFunc
	g              *errgroup.Group
}

// RunInProcess starts a plugin in-process for integration testing.
// Call Stop() when done to shut down the plugin.
func RunInProcess(plugin pluginruntime.Plugin) (*InProcessPlugin, error) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	telemetry := observability.NewTelemetryService(logger, nil, nil)

	h := &MockHost{
		logger:    logger,
		telemetry: telemetry,
	}

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	g, ctx := errgroup.WithContext(ctx)

	mux := http.NewServeMux()

	uninstallFn := func(ctx context.Context) error {
		installer, ok := plugin.(pluginruntime.Installer)
		if !ok {
			return nil
		}
		h.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseUninstalling, Message: "uninstalling"})
		if err := installer.Uninstall(ctx, h); err != nil {
			return err
		}
		cancel()
		return nil
	}

	handler := pluginruntime.NewMetadataHandler(
		func() pluginruntime.PluginStatus { return h.CurrentStatus() },
		func() pluginruntime.PluginDefinition { return plugin.Definition() },
		uninstallFn,
	)
	path, rpcHandler := pluginmetadatav1connect.NewPluginMetadataServiceHandler(handler)
	mux.Handle(path, rpcHandler)

	httpServer := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	g.Go(func() error {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("http serve: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		return httpServer.Shutdown(shutdownCtx)
	})

	g.Go(func() error {
		return plugin.Start(ctx, h)
	})

	baseURL := fmt.Sprintf("http://%s", listener.Addr().String())
	client := pluginmetadatav1connect.NewPluginMetadataServiceClient(http.DefaultClient, baseURL)

	return &InProcessPlugin{
		Plugin:         plugin,
		MetadataClient: client,
		BaseURL:        baseURL,
		cancel:         cancel,
		g:              g,
	}, nil
}

// Stop shuts down the in-process plugin.
func (p *InProcessPlugin) Stop() error {
	p.cancel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.Plugin.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("plugin shutdown: %w", err)
	}

	if err := p.g.Wait(); err != nil {
		return fmt.Errorf("plugin group: %w", err)
	}
	return nil
}
