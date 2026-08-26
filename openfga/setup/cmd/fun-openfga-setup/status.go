package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const statusPath = "/status.json"

// serveStatus publishes the provisioning result until the context is cancelled.
// It answers one path and nothing else: it runs beside the authorization server,
// and must never become a way to reach anything else in that pod.
func serveStatus(ctx context.Context, addr string, status Status) error {
	body, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("encode status: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc(statusPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	slog.Info("publishing provisioning status", "addr", addr, "generation", status.Generation)

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve status: %w", err)
	}

	return nil
}
