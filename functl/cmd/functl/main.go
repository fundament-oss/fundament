package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/alecthomas/kong"

	"github.com/fundament-oss/fundament/functl/pkg/cli"
	"github.com/fundament-oss/fundament/functl/pkg/config"
)

func main() {
	var root cli.CLI
	ctx := kong.Parse(&root,
		kong.Name("functl"),
		kong.Description("CLI for interacting with the Fundament platform."),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			NoExpandSubcommands: true,
		}),
	)

	logLevel := slog.LevelInfo
	if root.Debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))

	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load config: %v\n", err)
		os.Exit(1)
	}

	runCtx := &cli.Context{
		Debug:  root.Debug,
		Output: root.Output,
		Logger: logger,
		Config: cfg,
	}

	err = ctx.Run(runCtx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
