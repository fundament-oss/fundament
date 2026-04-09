package main

import (
	"fmt"
	"log/slog"
	"os"

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

	runCtx := &cli.Context{
		Debug:  root.Debug,
		Output: root.Output,
		Logger: logger,
	}

	code := root.Run(runCtx)
	if code == cli.ExitViolation {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "To allow a fresh dependency, add it to .gomod-age.json:")
		fmt.Fprintln(os.Stderr, `  { "allow": [{ "module": "<module>", "version": "<version>", "reason": "<reason>" }] }`)
	}
	os.Exit(code)
}
