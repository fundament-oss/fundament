// Command fun-marketplace-admin-api serves admin.v1.ReviewService, the
// Fundament backoffice surface (FUN-20). It is a separate deployable so it can
// be restricted to the admin host without the developer or public surfaces
// inheriting the restriction.
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
	"github.com/fundament-oss/fundament/marketplace-admin-api/pkg/admin"
)

// Authenticated but never organization-scoped: a reviewer acts on submissions
// from every publishing organization, so there is no organization header and
// no per-request RLS context.
type config struct {
	Database   psqldb.Config
	ListenAddr string     `env:"LISTEN_ADDR" envDefault:":8080"`
	LogLevel   slog.Level `env:"LOG_LEVEL" envDefault:"info"`
	JWTSecret  string     `env:"JWT_SECRET,required,notEmpty"`
	// Served on /version so callers outside the cluster can tell which release
	// is answering; the previous one keeps serving until Flux reconciles.
	DeploymentVersion string `env:"DEPLOYMENT_VERSION" envDefault:"unknown"`
	// Required: this surface takes credentials, and an empty list would mean a
	// wildcard origin, which browsers reject on credentialed requests.
	CORSAllowedOrigins []string `env:"CORS_ALLOWED_ORIGINS,required,notEmpty"`
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// newHealthMux serves the probes. readyz reports the database because a pod
// that cannot reach it can serve nothing; livez deliberately does not, so a
// brief outage does not get the container killed and restarted.
func newHealthMux(logger *slog.Logger, deploymentVersion string, database *psqldb.DB) *http.ServeMux {
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
				// The probe is public and a pgx error names the host, port,
				// database and role, so the detail stays in the log.
				logger.ErrorContext(ctx, "readiness probe failed", "error", err)
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte("database unavailable"))
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

	logger.Info("starting marketplace-admin-api",
		"listen_addr", cfg.ListenAddr,
		"log_level", cfg.LogLevel.String(),
	)

	ctx := context.Background()

	// A plain pool: the admin role's policies are not organization-scoped, so
	// there is no per-request GUC to push, unlike the registry's.
	database, err := psqldb.New(ctx, logger, cfg.Database)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer database.Close()

	// Refuse to serve against a schema this build does not know about.
	dbversion.MustAssertLatestVersion(ctx, logger, database.Pool)

	server := admin.New(logger, admin.Config{
		JWTSecret:          []byte(cfg.JWTSecret),
		CORSAllowedOrigins: cfg.CORSAllowedOrigins,
	}, database)

	// Health endpoints sit on an outer mux so they bypass CORS and every
	// interceptor — a probe carries no JWT.
	outerMux := newHealthMux(logger, cfg.DeploymentVersion, database)
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
