// Package cli provides the command-line interface for gomod-age.
package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/fundament-oss/fundament/tools/gomod-age/pkg/checker"
	"github.com/fundament-oss/fundament/tools/gomod-age/pkg/config"
	"github.com/fundament-oss/fundament/tools/gomod-age/pkg/proxy"
)

// CLI defines the root command-line interface structure.
type CLI struct {
	Debug    bool         `help:"Enable debug logging."`
	Output   OutputFormat `help:"Output format: table or json." short:"o" default:"table" enum:"table,json"`
	Config   string       `help:"Path to config file." default:".gomod-age.json" type:"path"`
	Age      string       `help:"Minimum release age (e.g. 7d, 72h)." default:""`
	Indirect bool         `help:"Include indirect dependencies."`
	Ignore   string       `help:"Comma-separated glob patterns to ignore."`
}

// Context holds shared dependencies for command execution.
type Context struct {
	Debug  bool
	Output OutputFormat
	Logger *slog.Logger
}

const (
	ExitOK        = 0
	ExitViolation = 1
	ExitError     = 2
)

// Run executes the age check and returns an exit code.
func (c *CLI) Run(ctx *Context) int {
	cfg, err := config.Load(c.Config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return ExitError
	}

	// CLI flags override config
	if c.Age != "" {
		cfg.Age = c.Age
	}
	if c.Indirect {
		cfg.Indirect = true
	}

	// Append CLI ignore patterns to config
	if c.Ignore != "" {
		for _, p := range strings.Split(c.Ignore, ",") {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				cfg.Ignore = append(cfg.Ignore, trimmed)
			}
		}
	}

	// Auto-add GOPRIVATE to ignore list
	if goprivate := os.Getenv("GOPRIVATE"); goprivate != "" {
		for _, p := range strings.Split(goprivate, ",") {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				cfg.Ignore = append(cfg.Ignore, trimmed)
			}
		}
	}

	if err := cfg.Resolve(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return ExitError
	}

	ctx.Logger.Debug("configuration loaded",
		"min_age", cfg.MinAge,
		"indirect", cfg.Indirect,
		"ignore", cfg.Ignore,
		"allow_count", len(cfg.Allow),
	)

	proxyClient := proxy.NewClient(nil)
	results, err := checker.Check(context.Background(), cfg, proxyClient, time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return ExitError
	}

	if ctx.Output == OutputJSON {
		if err := PrintJSON(results); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return ExitError
		}
	} else {
		if err := printTable(results); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return ExitError
		}
	}

	if len(results.Errors) > 0 {
		return ExitError
	}
	if len(results.Violations) > 0 {
		return ExitViolation
	}
	return ExitOK
}

func printTable(results *checker.Results) error {
	if len(results.Violations) > 0 {
		if _, err := fmt.Fprintf(os.Stdout, "VIOLATIONS (%d):\n", len(results.Violations)); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
		w := NewTableWriter()
		if _, err := fmt.Fprintln(w, "MODULE\tVERSION\tPUBLISHED\tAGE\tREMAINING"); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
		for i := range results.Violations {
			v := &results.Violations[i]
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				v.Module, v.Version, v.PublishTime.Format(TimeFormat), v.Age, v.Remaining); err != nil {
				return fmt.Errorf("writing output: %w", err)
			}
		}
		if err := w.Flush(); err != nil {
			return fmt.Errorf("flushing output: %w", err)
		}
		fmt.Println()
	}

	if len(results.Errors) > 0 {
		if _, err := fmt.Fprintf(os.Stdout, "ERRORS (%d):\n", len(results.Errors)); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
		for i := range results.Errors {
			e := &results.Errors[i]
			if _, err := fmt.Fprintf(os.Stdout, "  %s@%s: %s\n", e.Module, e.Version, e.Reason); err != nil {
				return fmt.Errorf("writing output: %w", err)
			}
		}
		fmt.Println()
	}

	if len(results.Skipped) > 0 {
		if _, err := fmt.Fprintf(os.Stdout, "SKIPPED (%d):\n", len(results.Skipped)); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
		for i := range results.Skipped {
			s := &results.Skipped[i]
			if _, err := fmt.Fprintf(os.Stdout, "  %s@%s: %s\n", s.Module, s.Version, s.Reason); err != nil {
				return fmt.Errorf("writing output: %w", err)
			}
		}
		fmt.Println()
	}

	if _, err := fmt.Fprintf(os.Stdout, "Summary: %d passed, %d violations, %d skipped, %d errors\n",
		len(results.Passed), len(results.Violations), len(results.Skipped), len(results.Errors)); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}
	return nil
}
