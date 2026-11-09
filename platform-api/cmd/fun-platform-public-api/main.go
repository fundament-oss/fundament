// Command fun-platform-public-api serves platform.v1.OverviewService, the
// anonymous public platform page (FUN-23). Its database role can run
// tenant.platform_overview() and read no table, so even a compromised handler
// sees organization names and counts and nothing else.
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/caarlos0/env/v11"

	"github.com/fundament-oss/fundament/common/dbversion"
	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/platform-api/pkg/health"
	"github.com/fundament-oss/fundament/platform-api/pkg/overview"
)

type config struct {
	Database   psqldb.Config
	ListenAddr string     `env:"LISTEN_ADDR" envDefault:":8080"`
	LogLevel   slog.Level `env:"LOG_LEVEL" envDefault:"info"`
	// The console origin. Anonymous, so no credentials are allowed.
	CORSAllowedOrigins []string      `env:"CORS_ALLOWED_ORIGINS"`
	CacheTTL           time.Duration `env:"CACHE_TTL" envDefault:"60s"`
	DeploymentVersion  string        `env:"DEPLOYMENT_VERSION" envDefault:"unknown"`
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

	logger.Info("starting platform-public-api",
		"listen_addr", cfg.ListenAddr,
		"log_level", cfg.LogLevel.String(),
		"cache_ttl", cfg.CacheTTL,
	)

	ctx := context.Background()

	// No connection hooks: the page is anonymous, so there is no organization
	// or user to push into the session. The function-only role is the boundary.
	database, err := psqldb.New(ctx, logger, cfg.Database)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer database.Close()

	dbversion.MustAssertLatestVersion(ctx, logger, database.Pool)

	server := overview.New(logger, database.Pool, overview.Config{
		CORSAllowedOrigins: cfg.CORSAllowedOrigins,
		CacheTTL:           cfg.CacheTTL,
	})

	mux := health.NewMux(logger, cfg.DeploymentVersion, database.Pool)
	mux.Handle("/", server.Handler())

	logger.Info("server listening", "addr", cfg.ListenAddr)
	if err := health.ListenAndServe(cfg.ListenAddr, mux); err != nil {
		return fmt.Errorf("server failed: %w", err)
	}
	return nil
}
