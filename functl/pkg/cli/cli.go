// Package cli provides the command-line interface for the Fundament CLI.
package cli

import (
	"log/slog"

	"github.com/fundament-oss/fundament/functl/pkg/client"
	"github.com/fundament-oss/fundament/functl/pkg/config"
)

// CLI defines the root command-line interface structure.
type CLI struct {
	Debug  bool         `help:"Enable debug logging."`
	Output OutputFormat `help:"Output format: table or json." short:"o" default:"table" enum:"table,json"`

	Auth      AuthCmd      `cmd:"" help:"Authentication commands."`
	Cluster   ClusterCmd   `cmd:"" help:"Manage clusters."`
	Project   ProjectCmd   `cmd:"" help:"Manage projects."`
	Namespace NamespaceCmd `cmd:"" help:"Manage namespaces."`
	APIKey    APIKeyCmd    `cmd:"" name:"apikey" help:"Manage API keys."`
}

// Context holds shared dependencies for command execution.
type Context struct {
	Debug  bool
	Output OutputFormat
	Logger *slog.Logger
	Config *config.Config
	Client *client.Client
}

// NewClientFromConfig creates a new API client from configuration.
// Returns an error if not authenticated.
func NewClientFromConfig() (*client.Client, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}

	creds, err := config.LoadCredentials()
	if err != nil {
		return nil, err
	}

	return client.New(creds.APIKey, cfg.APIEndpoint, cfg.AuthnURL), nil
}
