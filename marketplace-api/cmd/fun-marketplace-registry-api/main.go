// Command fun-marketplace-registry-api serves registry.v1.PublicationService,
// the plugin developer's publishing surface (FUN-20).
package main

import (
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/caarlos0/env/v11"
)

// Unlike the catalog, this surface is authenticated: every RPC is scoped to the
// caller's organization, so JWT_SECRET lands with the auth interceptor in the
// service implementation.
type config struct {
	ListenAddr string     `env:"LISTEN_ADDR" envDefault:":8080"`
	LogLevel   slog.Level `env:"LOG_LEVEL" envDefault:"info"`
	// Served on /version so callers outside the cluster can tell which release
	// is answering; the previous one keeps serving until Flux reconciles.
	DeploymentVersion string `env:"DEPLOYMENT_VERSION" envDefault:"unknown"`
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// newHealthMux serves the probes only. The database connection, the auth
// interceptor and the registry.v1 handlers land with the service
// implementation; until then this deploys green and reserves the image, chart
// and route.
func newHealthMux(deploymentVersion string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(deploymentVersion))
	})
	return mux
}

func run() error {
	var cfg config
	if err := env.Parse(&cfg); err != nil {
		return fmt.Errorf("env parse: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
	slog.SetDefault(logger)

	logger.Info("starting marketplace-registry-api",
		"listen_addr", cfg.ListenAddr,
		"log_level", cfg.LogLevel.String(),
	)

	// Cleartext HTTP/2 with prior knowledge: the ingress speaks h2c to the pod.
	// Uses the stdlib rather than x/net/http2/h2c, whose Upgrade: handshake
	// nothing here uses.
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           newHealthMux(cfg.DeploymentVersion),
		Protocols:         protocols,
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info("server listening", "addr", cfg.ListenAddr)
	if err := httpServer.ListenAndServe(); err != nil {
		return fmt.Errorf("server failed: %w", err)
	}

	return nil
}
