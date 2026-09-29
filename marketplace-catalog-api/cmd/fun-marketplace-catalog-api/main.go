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
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fundament-oss/fundament/common/dbversion"
	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/catalog"
	"github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/install"
)

// The storefront (catalog.v1) is unauthenticated by design (FUN-20) and reads
// through Database. install.v1 (FUN-22) is served from the same binary and
// reads through InstallDatabase as the tenant-aware install role; JWT_SECRET
// is for that surface only and, being configuration, is required up front;
// the install database itself is a runtime dependency and is connected lazily.
type config struct {
	Database        psqldb.Config
	InstallDatabase psqldb.Config `envPrefix:"INSTALL_"`
	JWTSecret       string        `env:"JWT_SECRET,required,notEmpty"`
	// Origins allowed to call install.v1 with credentials (the console);
	// separate from the storefront's anonymous CORS list.
	InstallCORSAllowedOrigins []string   `env:"INSTALL_CORS_ALLOWED_ORIGINS"`
	ListenAddr                string     `env:"LISTEN_ADDR" envDefault:":8080"`
	LogLevel                  slog.Level `env:"LOG_LEVEL" envDefault:"info"`
	CORSAllowedOrigins        []string   `env:"CORS_ALLOWED_ORIGINS"`
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

	installDB, err := install.NewDB(ctx, logger, cfg.InstallDatabase)
	if err != nil {
		return fmt.Errorf("failed to setup install database: %w", err)
	}
	defer installDB.Close()

	installServer := install.New(logger, install.Config{
		JWTSecret:          []byte(cfg.JWTSecret),
		CORSAllowedOrigins: cfg.InstallCORSAllowedOrigins,
	}, installDB)

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
	// Readiness is the storefront's alone: the install pool is lazy and its
	// database may lag a chart upgrade; taking the anonymous catalog out of
	// rotation for that would be the wrong trade (FUN-22 review).
	outerMux.HandleFunc("/readyz", readyz(logger, db.Pool))
	// install.v1 sits next to the storefront on the same host with its own
	// (credentialed) CORS policy; the more specific pattern wins over "/".
	outerMux.Handle(installServer.Path(), installServer.Handler())
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

// The probe is anonymous and publicly routed, and a pgx error names the host,
// port, database and role, so the detail goes to the log and never to the body.
func readyz(logger *slog.Logger, pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := pool.Ping(ctx); err != nil {
			logger.ErrorContext(ctx, "readiness probe failed", "error", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("database unavailable"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}
