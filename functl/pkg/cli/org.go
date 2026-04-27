package cli

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"github.com/fundament-oss/fundament/functl/pkg/config"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// OrgCmd contains organization subcommands.
type OrgCmd struct {
	List   OrgListCmd   `cmd:"" help:"List organizations."`
	Set    OrgSetCmd    `cmd:"" help:"Set the active organization used for subsequent commands."`
	Unset  OrgUnsetCmd  `cmd:"" help:"Clear the active organization."`
	Member OrgMemberCmd `cmd:"" help:"Manage organization members."`
}

// OrgListCmd handles listing organizations the current user belongs to.
type OrgListCmd struct{}

// Run executes the org list command.
func (c *OrgListCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig()
	if err != nil {
		return err
	}

	resp, err := apiClient.Organizations().ListOrganizations(context.Background(), connect.NewRequest(organizationv1.ListOrganizationsRequest_builder{}.Build()))
	if err != nil {
		return fmt.Errorf("failed to list organizations: %w", err)
	}

	orgs := resp.Msg.GetOrganizations()

	if ctx.Output == OutputJSON {
		return PrintJSON(orgs)
	}

	if len(orgs) == 0 {
		fmt.Println("No organizations found")
		return nil
	}

	w := NewTableWriter()
	fmt.Fprintln(w, "ID\tNAME\tALIAS\tCREATED")
	for _, org := range orgs {
		created := ""
		if org.GetCreated().IsValid() {
			created = org.GetCreated().AsTime().Format(TimeFormat)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			org.GetId(),
			org.GetName(),
			org.GetAlias(),
			created,
		)
	}
	return w.Flush()
}

// OrgSetCmd handles selecting the active organization.
type OrgSetCmd struct {
	OrgID string `arg:"" help:"Organization ID to use as the default for subsequent commands." name:"org-id"`
}

// Run executes the org set command.
func (c *OrgSetCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig()
	if err != nil {
		return err
	}

	resp, err := apiClient.Organizations().ListOrganizations(context.Background(), connect.NewRequest(organizationv1.ListOrganizationsRequest_builder{}.Build()))
	if err != nil {
		return fmt.Errorf("failed to list organizations: %w", err)
	}

	var match *organizationv1.Organization
	for _, org := range resp.Msg.GetOrganizations() {
		if org.GetId() == c.OrgID {
			match = org
			break
		}
	}
	if match == nil {
		return fmt.Errorf("organization %q not found or not accessible", c.OrgID)
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	cfg.Organization = match.GetId()
	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Now using organization: %s (%s)\n", match.GetAlias(), match.GetId())
	return nil
}

// OrgUnsetCmd handles clearing the active organization.
type OrgUnsetCmd struct{}

// Run executes the org unset command.
func (c *OrgUnsetCmd) Run(ctx *Context) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}

	if cfg.Organization == "" {
		fmt.Println("No active organization is set.")
		return nil
	}

	previous := cfg.Organization
	cfg.Organization = ""
	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Cleared active organization (was %s).\n", previous)
	return nil
}
