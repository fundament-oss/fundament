// Command fun-marketplace-registry-api serves registry.v1.PublicationService,
// the plugin developer's publishing surface (FUN-20).
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
	"github.com/fundament-oss/fundament/marketplace-api/pkg/registry"
)

// Unlike the catalog, this surface is authenticated: every RPC is scoped to the
// caller's organization, so it holds a JWT secret.
type config struct {
	Database   psqldb.Config
	ListenAddr string     `env:"LISTEN_ADDR" envDefault:":8080"`
	LogLevel   slog.Level `env:"LOG_LEVEL" envDefault:"info"`
	JWTSecret  string     `env:"JWT_SECRET,required,notEmpty"`
	// Served on /version so callers outside the cluster can tell which release
	// is answering; the previous one keeps serving until Flux reconciles.
	DeploymentVersion  string   `env:"DEPLOYMENT_VERSION" envDefault:"unknown"`
	CORSAllowedOrigins []string `env:"CORS_ALLOWED_ORIGINS"`
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// newHealthMux serves the probes. readyz reports the database because a pod
// that cannot reach it can serve no RPC; livez deliberately does not, so a brief
// database outage does not get the container killed and restarted.
func newHealthMux(deploymentVersion string, database *psqldb.DB) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if database != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			if err := database.Pool.Ping(ctx); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte("database: " + err.Error()))
				return
			}
		}
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

	ctx := context.Background()

	database, err := registry.NewDB(ctx, logger, cfg.Database)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer database.Close()

	// Refuse to serve against a schema this build does not know about.
	dbversion.MustAssertLatestVersion(ctx, logger, database.Pool)

	server := registry.New(logger, registry.Config{
		JWTSecret:          []byte(cfg.JWTSecret),
		CORSAllowedOrigins: cfg.CORSAllowedOrigins,
	}, database)

	// Health endpoints sit on an outer mux so they bypass CORS and every
	// interceptor — a probe carries no JWT.
	outerMux := newHealthMux(cfg.DeploymentVersion, database)
	outerMux.Handle("/", server.Handler())

	// Cleartext HTTP/2 with prior knowledge: the ingress speaks h2c to the pod.
	// Uses the stdlib rather than x/net/http2/h2c, whose Upgrade: handshake
	// nothing here uses.
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
