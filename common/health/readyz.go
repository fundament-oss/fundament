// Package health provides the shared /readyz handler: every service pings the
// dependencies it cannot serve without, and the probe is public, so failures
// name only the dependency — the error detail goes to the log, never the body
// (a pgx error names the host, port, database and role).
package health

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Check names one readiness dependency. Probe returns an error when the
// dependency cannot serve; pgxpool.Pool.Ping and similar methods satisfy it
// directly.
type Check struct {
	Name  string
	Probe func(ctx context.Context) error
}

// Readyz answers 200 "ok" when every check passes, and 503 naming only the
// failed dependencies otherwise. Each request gets a 5-second budget; the
// probe's kubelet timeout must sit above it (fundament.readinessProbe sets 6s)
// or a slow-but-successful ping reads as a failure.
func Readyz(logger *slog.Logger, checks ...Check) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		var failed []string
		for _, check := range checks {
			if err := check.Probe(ctx); err != nil {
				logger.ErrorContext(ctx, "readiness probe failed", "check", check.Name, "error", err)
				failed = append(failed, check.Name+" unavailable")
			}
		}
		if len(failed) > 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(strings.Join(failed, "\n")))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}
