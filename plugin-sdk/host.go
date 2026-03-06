package pluginsdk

import (
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/fundament-oss/fundament/plugin-sdk/observability"
)

// Host is provided by the Run harness to give plugins access to framework services.
type Host interface {
	// Logger returns the structured logger for this plugin.
	Logger() *slog.Logger

	// Telemetry returns the telemetry service for tracing and metrics.
	Telemetry() observability.TelemetryService

	// ReportStatus updates the plugin's current status, which is served
	// via the metadata API.
	ReportStatus(status PluginStatus)

	// ReportReady signals that the plugin is ready to serve traffic.
	// This flips the readiness probe to healthy.
	ReportReady()
}

type host struct {
	logger    *slog.Logger
	telemetry observability.TelemetryService
	ready     atomic.Bool
	status    PluginStatus
	statusMu  sync.RWMutex
}

func newHost(logger *slog.Logger, telemetry observability.TelemetryService) *host {
	return &host{
		logger:    logger,
		telemetry: telemetry,
		status: PluginStatus{
			Phase: PhaseInstalling,
		},
	}
}

func (h *host) Logger() *slog.Logger {
	return h.logger
}

func (h *host) Telemetry() observability.TelemetryService {
	return h.telemetry
}

func (h *host) ReportStatus(status PluginStatus) {
	h.statusMu.Lock()
	defer h.statusMu.Unlock()
	h.status = status
	h.logger.Info("plugin status changed", "phase", status.Phase, "message", status.Message)
}

func (h *host) ReportReady() {
	h.ready.Store(true)
	h.logger.Info("plugin reported ready")
}

// IsReady implements health.ReadinessChecker.
func (h *host) IsReady() bool {
	return h.ready.Load()
}

// CurrentStatus implements metadata.StatusProvider.
func (h *host) CurrentStatus() PluginStatus {
	h.statusMu.RLock()
	defer h.statusMu.RUnlock()
	return h.status
}
