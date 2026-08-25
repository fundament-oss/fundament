package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/caarlos0/env/v11"

	"github.com/fundament-oss/fundament/common/dbversion"
	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/marketplace-api/pkg/catalog"
)

// No JWT_SECRET: the storefront is unauthenticated by design (FUN-20).
type config struct {
	Database           psqldb.Config
	ListenAddr         string     `env:"LISTEN_ADDR" envDefault:":8080"`
	LogLevel           slog.Level `env:"LOG_LEVEL" envDefault:"info"`
	CORSAllowedOrigins []string   `env:"CORS_ALLOWED_ORIGINS"`
	// Served on /version so callers outside the cluster can tell which release
	// is answering; the previous one keeps serving until Flux reconciles.
	DeploymentVersion string `env:"DEPLOYMENT_VERSION" envDefault:"unknown"`
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
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

	logger.Info("starting marketplace-catalog-api",
		"listen_addr", cfg.ListenAddr,
		"log_level", cfg.LogLevel.String(),
	)

	ctx := context.Background()

	db, err := catalog.NewDB(ctx, logger, cfg.Database)
	if err != nil {
		return fmt.Errorf("failed to setup database: %w", err)
	}

	defer db.Close()

	dbversion.MustAssertLatestVersion(ctx, logger, db.Pool)

	server := catalog.New(logger, db, cfg.CORSAllowedOrigins)

	// Health endpoints are registered on an outer mux so they bypass CORS
	// and Connect interceptors.
	outerMux := http.NewServeMux()
	outerMux.HandleFunc("/livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	outerMux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(cfg.DeploymentVersion))
	})
	outerMux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := db.Pool.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("database: " + err.Error()))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	outerMux.Handle("/", server.Handler())

	// Cleartext HTTP/2 with prior knowledge: the ingress speaks h2c to the pod.
	// Replaces x/net/http2/h2c, whose Upgrade: handshake nothing here uses.
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           outerMux,
		Protocols:         protocols,
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info("server listening", "addr", cfg.ListenAddr)
	if err := httpServer.ListenAndServe(); err != nil {
		return fmt.Errorf("server failed: %w", err)
	}

	return nil
}
