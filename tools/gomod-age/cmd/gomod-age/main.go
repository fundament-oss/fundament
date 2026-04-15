package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"

	"github.com/fundament-oss/fundament/tools/gomod-age/pkg/cli"
)

func main() {
	var root cli.CLI
	kong.Parse(&root,
		kong.Name("gomod-age"),
		kong.Description("Check Go module dependencies for minimum release age."),
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

	env := &cli.Env{
		Debug:  root.Debug,
		Output: root.Output,
		Logger: logger,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	code := root.Run(ctx, env)
	if code == cli.ExitViolation {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "To allow a fresh dependency, add it to .gomod-age.json:")
		fmt.Fprintln(os.Stderr, `  { "allow": [{ "module": "<module>", "version": "<version>", "reason": "<reason>" }] }`)
	}
	os.Exit(code)
}
