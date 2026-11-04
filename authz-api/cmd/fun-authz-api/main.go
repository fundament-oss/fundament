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

	"github.com/fundament-oss/fundament/authz-api/pkg/authzapi"
	"github.com/fundament-oss/fundament/common/authz"
)

type config struct {
	OpenFGA            authz.Config
	JWTSecret          string     `env:"JWT_SECRET,required,notEmpty"`
	ListenAddr         string     `env:"LISTEN_ADDR" envDefault:":8080"`
	LogLevel           slog.Level `env:"LOG_LEVEL" envDefault:"info"`
	CORSAllowedOrigins []string   `env:"CORS_ALLOWED_ORIGINS"`
	// Served on /version so callers outside the cluster can tell which release is answering.
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

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	logger.Info("starting authz-api",
		"listen_addr", cfg.ListenAddr,
		"log_level", cfg.LogLevel.String(),
		"openfga_api_url", cfg.OpenFGA.APIURL,
		"openfga_store_name", cfg.OpenFGA.StoreName,
	)

	authzStore, err := authz.NewStore(cfg.OpenFGA)
	if err != nil {
		return fmt.Errorf("failed to create OpenFGA client: %w", err)
	}

	server := authzapi.New(logger, &authzapi.Config{
		JWTSecret:          []byte(cfg.JWTSecret),
		CORSAllowedOrigins: cfg.CORSAllowedOrigins,
	}, authz.NewClient(authzStore))

	// Health endpoints sit on an outer mux so they bypass CORS and Connect interceptors.
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

		if err := authzStore.Healthy(ctx); err != nil {
			logger.WarnContext(ctx, "readiness check failed", "error", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("openfga: unavailable"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	outerMux.Handle("/", server.Handler())

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
