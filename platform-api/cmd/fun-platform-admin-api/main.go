// Command fun-platform-admin-api serves fleet.v1.FleetService, the staff view
// of every organization's clusters (FUN-23). It is a separate deployable from
// the public overview so it can be restricted to the admin host (FUN-20), and
// it accepts only the DCIM staff token.
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/caarlos0/env/v11"

	"github.com/fundament-oss/fundament/common/dbversion"
	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/platform-api/pkg/fleet"
	"github.com/fundament-oss/fundament/platform-api/pkg/health"
)

// Authenticated but never organization-scoped: the role's policies read every
// organization, so there is no per-request RLS context to push.
type config struct {
	Database   psqldb.Config
	ListenAddr string     `env:"LISTEN_ADDR" envDefault:":8080"`
	LogLevel   slog.Level `env:"LOG_LEVEL" envDefault:"info"`
	JWTSecret  string     `env:"JWT_SECRET,required,notEmpty"`
	// Required: this surface takes credentials, and an empty list would mean a
	// wildcard origin, which browsers reject on credentialed requests.
	CORSAllowedOrigins []string `env:"CORS_ALLOWED_ORIGINS,required,notEmpty"`
	DeploymentVersion  string   `env:"DEPLOYMENT_VERSION" envDefault:"unknown"`
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

	logger.Info("starting platform-admin-api",
		"listen_addr", cfg.ListenAddr,
		"log_level", cfg.LogLevel.String(),
	)

	ctx := context.Background()

	database, err := psqldb.New(ctx, logger, cfg.Database)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer database.Close()

	// Refuse to serve against a schema this build does not know about.
	dbversion.MustAssertLatestVersion(ctx, logger, database.Pool)

	server := fleet.New(logger, fleet.Config{
		JWTSecret:          []byte(cfg.JWTSecret),
		CORSAllowedOrigins: cfg.CORSAllowedOrigins,
	}, database)

	mux := health.NewMux(logger, cfg.DeploymentVersion, database.Pool)
	mux.Handle("/", server.Handler())

	logger.Info("server listening", "addr", cfg.ListenAddr)
	if err := health.ListenAndServe(cfg.ListenAddr, mux); err != nil {
		return fmt.Errorf("server failed: %w", err)
	}
	return nil
}
