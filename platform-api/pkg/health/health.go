// Package health serves the probes both platform-api binaries share.
package health

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewMux serves /livez, /readyz and /version. readyz reports the database
// because a pod that cannot reach it can serve nothing; livez deliberately
// does not, so a brief outage does not get the container restarted. The mux
// sits outside CORS and every interceptor: a probe carries no credentials.
func NewMux(logger *slog.Logger, deploymentVersion string, pool *pgxpool.Pool) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if pool != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			if err := pool.Ping(ctx); err != nil {
				// The probe is publicly routed and a pgx error names the host,
				// port, database and role, so the detail stays in the log.
				logger.ErrorContext(ctx, "readiness probe failed", "error", err)
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte("database unavailable"))
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	// Served so callers outside the cluster can tell which release is
	// answering; the previous one keeps serving until Flux reconciles.
	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(deploymentVersion))
	})
	return mux
}

// ListenAndServe serves handler on addr with cleartext HTTP/2 with prior
// knowledge next to HTTP/1: the gateway speaks h2c to the pod.
func ListenAndServe(addr string, handler http.Handler) error {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		Protocols:         protocols,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return server.ListenAndServe() //nolint:wrapcheck // callers wrap
}
