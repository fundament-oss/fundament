// Package cli provides the command-line interface for funops.
package cli

import (
	"context"
	"log/slog"

	"github.com/fundament-oss/fundament/common/psqldb"
	db "github.com/fundament-oss/fundament/funops/pkg/db/gen"
)

// CLI defines the root command-line interface structure for funops.
type CLI struct {
	Debug  bool         `help:"Enable debug logging."`
	Output OutputFormat `help:"Output format: table or json." short:"o" default:"table" enum:"table,json"`

	Cluster      ClusterCmd      `cmd:"" help:"Manage clusters."`
	Namespace    NamespaceCmd    `cmd:"" help:"Manage namespaces."`
	Organization OrganizationCmd `cmd:"" help:"Manage organizations and their members."`
	Plugin       PluginCmd       `cmd:"" help:"Review submitted plugin versions."`
	User         UserCmd         `cmd:"" help:"Manage users."`
}

// Context holds shared dependencies for command execution.
type Context struct {
	Debug  bool
	Output OutputFormat
	Logger *slog.Logger
	DB     *psqldb.DB
	// Ctx is cancelled on interrupt, so a command waiting on the database
	// stops instead of running on after the operator gave up.
	Ctx     context.Context
	Queries *db.Queries
}
