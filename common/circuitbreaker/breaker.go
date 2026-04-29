package circuitbreaker

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

// TripCheck reports whether the circuit should be open (tripped).
// Returns true to trip the breaker, false to keep it closed.
type TripCheck func(ctx context.Context) (bool, error)

// Config holds circuit breaker settings.
type Config struct {
	PollInterval time.Duration
}

// Breaker periodically runs a check and trips when the check says so.
type Breaker struct {
	config Config
	logger *slog.Logger
	check  TripCheck
	open   atomic.Bool
}

// New creates a Breaker. Default poll interval: 2s.
func New(logger *slog.Logger, cfg Config, check TripCheck) *Breaker {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 2 * time.Second
	}
	return &Breaker{
		config: cfg,
		logger: logger,
		check:  check,
	}
}

// IsOpen returns true when the breaker is tripped.
func (b *Breaker) IsOpen() bool {
	return b.open.Load()
}

// Start polls the check on the configured interval until ctx is cancelled.
func (b *Breaker) Start(ctx context.Context) {
	ticker := time.NewTicker(b.config.PollInterval)
	defer ticker.Stop()

	// Check immediately on start.
	b.poll(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.poll(ctx)
		}
	}
}

func (b *Breaker) poll(ctx context.Context) {
	shouldOpen, err := b.check(ctx)
	if err != nil {
		b.logger.WarnContext(ctx, "circuit breaker check failed", "error", err)
		// Fail-open: don't block traffic because of a check failure.
		return
	}

	// When shouldOpen is true, we try CompareAndSwap(false, true) — this atomically checks "is it currently false (closed)?" and only if so, flips it to true (open).
	// The swap succeeding means we just transitioned from closed to open, so we log it.
	// When shouldOpen is false, we try CompareAndSwap(true, false) — "is it currently true (open)?" and if so, flip to false (closed).
	// Again, success means a state transition worth logging.
	if shouldOpen {
		if b.open.CompareAndSwap(false, true) {
			b.logger.WarnContext(ctx, "circuit breaker OPEN")
		}
	} else {
		if b.open.CompareAndSwap(true, false) {
			b.logger.InfoContext(ctx, "circuit breaker CLOSED")
		}
	}
}
