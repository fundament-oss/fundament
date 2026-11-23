package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/alecthomas/kong"

	"github.com/fundament-oss/fundament/common/dbversion"
	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/funops/pkg/cli"
	db "github.com/fundament-oss/fundament/funops/pkg/db/gen"
)

func main() {
	var root cli.CLI
	ctx := kong.Parse(&root,
		kong.Name("funops"),
		kong.Description("Operator CLI for Fundament platform administration."),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			NoExpandSubcommands: true,
		}),
	)

	if err := run(ctx, &root); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx *kong.Context, root *cli.CLI) error {
	// Set up logging - default to INFO for user feedback
	logLevel := slog.LevelInfo
	if root.Debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))

	// Get database URL from environment
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("DATABASE_URL environment variable is required")
	}

	// Connect to database
	bgCtx := context.Background()
	database, err := psqldb.New(bgCtx, logger, psqldb.Config{URL: databaseURL})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer database.Close()

	if err := dbversion.AssertLatestVersion(bgCtx, database.Pool); err != nil {
		return fmt.Errorf("database schema version check failed: %w", err)
	}

	queries := db.New(database.Pool)

	runCtx := &cli.Context{
		Debug:   root.Debug,
		Output:  root.Output,
		Logger:  logger,
		DB:      database,
		Queries: queries,
	}

	return ctx.Run(runCtx) //nolint:wrapcheck // printed to the operator unchanged
}
