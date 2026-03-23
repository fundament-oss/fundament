package observability

import (
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// TelemetryService provides access to structured logging, tracing, and metrics.
type TelemetryService interface {
	Logger() *slog.Logger
	TracerProvider() trace.TracerProvider
	MeterProvider() metric.MeterProvider
}

type telemetryService struct {
	logger         *slog.Logger
	tracerProvider trace.TracerProvider
	meterProvider  metric.MeterProvider
}

// NewTelemetryService creates a TelemetryService. If tracerProvider or meterProvider
// are nil, the OTel global providers are used (which respect OTEL_* env vars).
func NewTelemetryService(logger *slog.Logger, tp trace.TracerProvider, mp metric.MeterProvider) TelemetryService {
	if tp == nil {
		tp = otel.GetTracerProvider()
	}
	if mp == nil {
		mp = otel.GetMeterProvider()
	}
	return &telemetryService{
		logger:         logger,
		tracerProvider: tp,
		meterProvider:  mp,
	}
}

func (t *telemetryService) Logger() *slog.Logger {
	return t.logger
}

func (t *telemetryService) TracerProvider() trace.TracerProvider {
	return t.tracerProvider
}

func (t *telemetryService) MeterProvider() metric.MeterProvider {
	return t.meterProvider
}
