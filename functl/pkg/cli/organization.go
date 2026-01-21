package cli

import (
	"context"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"

	db "github.com/fundament-oss/fundament/functl/pkg/db/gen"
)

// OrganizationCmd groups organization-related commands.
type OrganizationCmd struct {
	Create OrganizationCreateCmd `cmd:"" help:"Create a new organization."`
	List   OrganizationListCmd   `cmd:"" help:"List all organizations."`
	Delete OrganizationDeleteCmd `cmd:"" help:"Delete an organization."`
}

// OrganizationCreateCmd creates a new organization.
type OrganizationCreateCmd struct {
	Name string `arg:"" help:"Organization name." required:""`
}

// OrganizationListCmd lists all organizations.
type OrganizationListCmd struct{}

// OrganizationDeleteCmd deletes an organization.
type OrganizationDeleteCmd struct {
	Name string `arg:"" help:"Organization name." required:""`
}

// Run executes the organization create command.
func (c *OrganizationCreateCmd) Run(ctx *Context) error {
	ctx.Logger.Debug("creating organization", "name", c.Name)

	org, err := ctx.Queries.OrganizationCreate(context.Background(), db.OrganizationCreateParams{
		Name: c.Name,
	})
	if err != nil {
		// Check for unique constraint violation
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == pgerrcode.UniqueViolation {
			return fmt.Errorf("organization '%s' already exists", c.Name)
		}
		return fmt.Errorf("failed to create organization: %w", err)
	}

	ctx.Logger.Debug("organization created", "id", org.ID.String())

	return outputOrganizationCreate(ctx.Output, org)
}

// Run executes the organization list command.
func (c *OrganizationListCmd) Run(ctx *Context) error {
	ctx.Logger.Debug("listing organizations")

	orgs, err := ctx.Queries.OrganizationList(context.Background())
	if err != nil {
		return fmt.Errorf("failed to list organizations: %w", err)
	}

	ctx.Logger.Debug("organizations listed", "count", len(orgs))

	return outputOrganizationList(ctx.Output, orgs)
}

// Run executes the organization delete command.
func (c *OrganizationDeleteCmd) Run(ctx *Context) error {
	ctx.Logger.Debug("deleting organization", "name", c.Name)

	rowsAffected, err := ctx.Queries.OrganizationDelete(context.Background(), db.OrganizationDeleteParams{
		Name: c.Name,
	})
	if err != nil {
		return fmt.Errorf("failed to delete organization: %w", err)
	}
	if rowsAffected != 1 {
		return fmt.Errorf("organization '%s' not found", c.Name)
	}

	ctx.Logger.Info("deleted organization", "name", c.Name)

	return nil
}

// organizationOutput is the JSON output structure for an organization.
type organizationOutput struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Created string `json:"created"`
}

// organizationCreateOutput is the JSON output structure for organization create.
type organizationCreateOutput struct {
	ID string `json:"id"`
}

func outputOrganizationCreate(format OutputFormat, org db.TenantOrganization) error {
	switch format {
	case OutputJSON:
		return PrintJSON(organizationCreateOutput{
			ID: org.ID.String(),
		})
	case OutputTable:
		fmt.Println(org.ID.String())
		return nil
	default:
		panic(fmt.Sprintf("unknown output format: %s", format))
	}
}

func outputOrganizationList(format OutputFormat, orgs []db.TenantOrganization) error {
	switch format {
	case OutputJSON:
		output := make([]organizationOutput, len(orgs))
		for i, org := range orgs {
			output[i] = organizationOutput{
				ID:      org.ID.String(),
				Name:    org.Name,
				Created: org.Created.Time.Format(TimeFormat),
			}
		}
		return PrintJSON(output)
	case OutputTable:
		w := NewTableWriter()
		fmt.Fprintln(w, "ID\tNAME\tCREATED")
		for _, org := range orgs {
			fmt.Fprintf(w, "%s\t%s\t%s\n",
				org.ID.String(),
				org.Name,
				org.Created.Time.Format(TimeFormat),
			)
		}
		return w.Flush()
	default:
		panic(fmt.Sprintf("unknown output format: %s", format))
	}
}
