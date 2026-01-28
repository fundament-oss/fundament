// Package cli provides the command-line interface for functl.
package cli

import (
	"log/slog"

	db "github.com/fundament-oss/fundament/functl/pkg/db/gen"
)

// CLI defines the root command-line interface structure for functl.
type CLI struct {
	Debug  bool         `help:"Enable debug logging."`
	Output OutputFormat `help:"Output format: table or json." short:"o" default:"table" enum:"table,json"`

	Organization OrganizationCmd `cmd:"" help:"Manage organizations."`
	User         UserCmd         `cmd:"" help:"Manage users."`
	Namespace    NamespaceCmd    `cmd:"" help:"Manage namespaces."`
}

// Context holds shared dependencies for command execution.
type Context struct {
	Debug   bool
	Output  OutputFormat
	Logger  *slog.Logger
	Queries *db.Queries
}
