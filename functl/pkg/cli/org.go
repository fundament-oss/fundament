package cli

import (
	"context"
	"fmt"
	"os"

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

	resp, err := apiClient.Organizations().ListOrganizations(context.Background(), organizationv1.ListOrganizationsRequest_builder{}.Build())
	if err != nil {
		return fmt.Errorf("failed to list organizations: %w", err)
	}

	orgs := resp.GetOrganizations()

	if ctx.Output == OutputJSON {
		return PrintJSON(orgs)
	}

	if len(orgs) == 0 {
		fmt.Println(NoOrganizationsHint)
		return nil
	}

	w := NewTableWriter()
	_, _ = fmt.Fprintln(w, "ID\tNAME\tALIAS\tCREATED")
	for _, org := range orgs {
		created := ""
		if org.GetCreated().IsValid() {
			created = org.GetCreated().AsTime().Format(TimeFormat)
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			org.GetId(),
			org.GetName(),
			org.GetAlias(),
			created,
		)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flushing output: %w", err)
	}
	return nil
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

	resp, err := apiClient.Organizations().ListOrganizations(context.Background(), organizationv1.ListOrganizationsRequest_builder{}.Build())
	if err != nil {
		return fmt.Errorf("failed to list organizations: %w", err)
	}

	if len(resp.GetOrganizations()) == 0 {
		return fmt.Errorf("organization %q not found or not accessible\n%s", c.OrgID, NoOrganizationsHint)
	}

	var match *organizationv1.Organization
	for _, org := range resp.GetOrganizations() {
		if org.GetId() == c.OrgID {
			match = org
			break
		}
	}
	if match == nil {
		return fmt.Errorf("organization %q not found or not accessible: run 'functl org list' to see your organizations", c.OrgID)
	}

	cfg, err := config.LoadFileConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	cfg.Organization = match.GetId()
	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Now using organization: %s (%s)\n", match.GetAlias(), match.GetId())
	if ctx.Config.URL != cfg.URL {
		saved := cfg.URL
		if saved == "" {
			saved = config.DefaultURL
		}
		fmt.Fprintf(os.Stderr, "Warning: the organization was checked against %s (from the environment), but it is saved in the config file, whose installation is %s.\n", ctx.Config.URL, saved)
	}
	return nil
}

// OrgUnsetCmd handles clearing the active organization.
type OrgUnsetCmd struct{}

// Run executes the org unset command.
func (c *OrgUnsetCmd) Run(ctx *Context) error {
	cfg, err := config.LoadFileConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
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
