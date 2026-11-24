// Command gendoc writes the outbox row's state diagram into the package
// README, between markers, so the drawing cannot drift from the table that
// enforces it. It runs from pkg/outbox via go generate.
package main

import (
	"fmt"
	"os"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/outbox"
)

const readme = "../../README.md"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	content, err := os.ReadFile(readme)
	if err != nil {
		return fmt.Errorf("read README: %w", err)
	}

	updated, err := outbox.ReplaceReadmeDiagram(string(content))
	if err != nil {
		return fmt.Errorf("render README diagram: %w", err)
	}
	if updated == string(content) {
		return nil
	}
	//nolint:gosec // the README is a checked-in source file, not a secret
	if err := os.WriteFile(readme, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("write README: %w", err)
	}
	return nil
}
