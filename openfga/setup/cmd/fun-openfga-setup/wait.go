package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// WaitConfig configures the waiting role.
type WaitConfig struct {
	StatusURL  string        `env:"OPENFGA_STATUS_URL,required,notEmpty"`
	Generation string        `env:"OPENFGA_GENERATION,required,notEmpty"`
	Timeout    time.Duration `env:"OPENFGA_TIMEOUT" envDefault:"5m"`
}

// Wait blocks until the provisioner reports the expected generation.
//
// The previous release's provisioner answers this endpoint too, with its own
// generation, so only a reply carrying this release's generation proves the
// datastore is the one this release provisioned.
func Wait(ctx context.Context, cfg WaitConfig) error {
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	slog.Info("waiting for the OpenFGA datastore",
		"generation", cfg.Generation, "status_url", cfg.StatusURL, "timeout", cfg.Timeout)

	httpClient := &http.Client{Timeout: 5 * time.Second}

	// Polling every couple of seconds, so report only when the answer changes:
	// every distinct reason for still waiting gets one line, and a wait that hangs
	// says why rather than going silent.
	var lastReason string

	for {
		status, err := fetchStatus(ctx, httpClient, cfg.StatusURL)

		reason := ""

		switch {
		case err != nil:
			reason = "status endpoint unreachable: " + err.Error()
		case status.Generation == cfg.Generation:
			slog.Info("datastore ready",
				"generation", status.Generation, "store", status.Store, "id", status.StoreID)

			return nil
		default:
			reason = fmt.Sprintf("openfga is serving generation %q, waiting for %q",
				status.Generation, cfg.Generation)
		}

		if reason != lastReason {
			slog.Info("still waiting for the OpenFGA datastore", "reason", reason)

			lastReason = reason
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("openfga did not report generation %q within %s (%s): %w",
				cfg.Generation, cfg.Timeout, lastReason, ctx.Err())
		case <-time.After(pollInterval):
		}
	}
}

func fetchStatus(ctx context.Context, httpClient *http.Client, url string) (Status, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return Status{}, fmt.Errorf("build request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return Status{}, fmt.Errorf("fetch status: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return Status{}, fmt.Errorf("status endpoint returned %d", resp.StatusCode)
	}

	var status Status
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return Status{}, fmt.Errorf("decode status: %w", err)
	}

	return status, nil
}
