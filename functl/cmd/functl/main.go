package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/alecthomas/kong"

	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/functl/pkg/cli"
	db "github.com/fundament-oss/fundament/functl/pkg/db/gen"
)

func main() {
	var root cli.CLI
	ctx := kong.Parse(&root,
		kong.Name("functl"),
		kong.Description("Operator CLI for Fundament platform administration."),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			NoExpandSubcommands: true,
		}),
	)

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
		fmt.Fprintln(os.Stderr, "Error: DATABASE_URL environment variable is required")
		os.Exit(1)
	}

	// Connect to database
	bgCtx := context.Background()
	database, err := psqldb.New(bgCtx, logger, psqldb.Config{URL: databaseURL})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	queries := db.New(database.Pool)

	runCtx := &cli.Context{
		Debug:   root.Debug,
		Output:  root.Output,
		Logger:  logger,
		Queries: queries,
	}

	err = ctx.Run(runCtx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
