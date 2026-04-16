package cli

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// OrgCmd contains organization subcommands.
type OrgCmd struct {
	List   OrgListCmd   `cmd:"" help:"List organizations."`
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
