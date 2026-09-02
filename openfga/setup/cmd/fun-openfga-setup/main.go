// Command fun-openfga-setup provisions the OpenFGA store for a release and
// publishes what it provisioned, so consumers read the store rather than deriving
// it and authz-worker can tell this release's datastore from the outgoing one.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/caarlos0/env/v11"
)

func main() {
	if err := run(); err != nil {
		slog.Error("openfga setup failed", "command", command(), "error", err)
		os.Exit(1)
	}
}

func run() error {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	if len(os.Args) < 2 {
		return errors.New("usage: fun-openfga-setup provision")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	switch os.Args[1] {
	case "provision":
		cfg, err := parse[ProvisionConfig]()
		if err != nil {
			return err
		}

		return Provision(ctx, cfg)
	default:
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
}

// command reports the subcommand, for logs written before or after parsing.
func command() string {
	if len(os.Args) < 2 {
		return "(none)"
	}

	return os.Args[1]
}

func parse[T any]() (*T, error) {
	cfg := new(T)
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse configuration: %w", err)
	}

	return cfg, nil
}
