package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/alecthomas/kong"

	"github.com/fundament-oss/fundament/functl/pkg/cli"
	"github.com/fundament-oss/fundament/functl/pkg/config"
	"github.com/fundament-oss/fundament/functl/pkg/scaffold"
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
		kong.Vars{"sdk_version": scaffold.DefaultSDKVersion},
	)

	logLevel := slog.LevelInfo
	if root.Debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))

	// auth status reports a config it cannot load itself, as JSON under -o json, and
	// runs with a nil Config.
	cfg, err := config.LoadConfig()
	if err != nil {
		if ctx.Command() != "auth status" {
			fmt.Fprintf(os.Stderr, "Error: failed to load config: %v\n", err)
			os.Exit(1)
		}
	} else {
		logger.Debug("resolved endpoints",
			"api_endpoint", cfg.APIEndpoint,
			"api_endpoint_from_env", os.Getenv(config.EnvAPIEndpoint) != "",
			"authn_url", cfg.AuthnURL,
			"authn_url_from_env", os.Getenv(config.EnvAuthnURL) != "")
	}

	runCtx := &cli.Context{
		Debug:  root.Debug,
		Output: root.Output,
		Org:    root.OrgOverride,
		Logger: logger,
		Config: cfg,
	}

	err = ctx.Run(runCtx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
