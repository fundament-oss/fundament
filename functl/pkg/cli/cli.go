// Package cli provides the command-line interface for the Fundament CLI.
package cli

import (
	"errors"
	"log/slog"

	"github.com/fundament-oss/fundament/functl/pkg/client"
	"github.com/fundament-oss/fundament/functl/pkg/config"
)

// ErrNoActiveOrganization is returned when a command requires an active
// organization but none is configured.
var ErrNoActiveOrganization = errors.New("no active organization: pass --org=<org-id> or run 'functl org set <org-id>' to select one")

// CLI defines the root command-line interface structure.
type CLI struct {
	Debug       bool         `help:"Enable debug logging."`
	Output      OutputFormat `help:"Output format: table or json." short:"o" default:"table" enum:"table,json"`
	OrgOverride string       `name:"org" help:"Organization ID (overrides the active organization configured via 'functl org set')."`

	Auth      AuthCmd      `cmd:"" help:"Authentication commands."`
	Cluster   ClusterCmd   `cmd:"" help:"Manage clusters."`
	Config    ConfigCmd    `cmd:"" help:"Configuration introspection."`
	Org       OrgCmd       `cmd:"" help:"Manage organization."`
	Project   ProjectCmd   `cmd:"" help:"Manage projects."`
	Namespace NamespaceCmd `cmd:"" help:"Manage namespaces."`
	APIKey    APIKeyCmd    `cmd:"" name:"apikey" help:"Manage API keys."`
}

// Context holds shared dependencies for command execution.
type Context struct {
	Debug  bool
	Output OutputFormat
	Org    string
	Logger *slog.Logger
	Config *config.Config
	Client *client.Client
}

// ClientOpt configures a client created by NewClientFromConfig.
type ClientOpt func(*clientOpts)

type clientOpts struct {
	organizationID string
}

// WithOrg sets the organization ID on the client.
func WithOrg(id string) ClientOpt {
	return func(o *clientOpts) {
		o.organizationID = id
	}
}

// NewClientFromConfig creates a new API client from configuration.
// Returns an error if not authenticated.
func NewClientFromConfig(opts ...ClientOpt) (*client.Client, error) {
	var o clientOpts
	for _, opt := range opts {
		opt(&o)
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}

	creds, err := config.LoadCredentials()
	if err != nil {
		return nil, err
	}

	orgID := o.organizationID
	if orgID == "" {
		orgID = cfg.Organization
	}

	return client.New(creds.APIKey, cfg.APIEndpoint, cfg.AuthnURL, orgID), nil
}

// NewClientFromConfigWithOrg creates a new API client scoped to the active organization.
// The org is resolved in this order:
//  1. ctx.Org (set from the --org flag)
//  2. ctx.Config.Organization (set via 'functl org set')
//
// Returns ErrNoActiveOrganization if neither is set.
func NewClientFromConfigWithOrg(ctx *Context) (*client.Client, error) {
	orgID := ctx.Org
	if orgID == "" && ctx.Config != nil {
		orgID = ctx.Config.Organization
	}
	if orgID == "" {
		return nil, ErrNoActiveOrganization
	}
	return NewClientFromConfig(WithOrg(orgID))
}
