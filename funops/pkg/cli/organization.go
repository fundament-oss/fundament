package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fundament-oss/fundament/common/rollback"
	db "github.com/fundament-oss/fundament/funops/pkg/db/gen"
)

// OrganizationCmd groups organization-related commands.
type OrganizationCmd struct {
	Create OrganizationCreateCmd `cmd:"" help:"Create a new organization."`
	List   OrganizationListCmd   `cmd:"" help:"List all organizations."`
	Delete OrganizationDeleteCmd `cmd:"" help:"Delete an organization, revoking its memberships and API keys."`
	Member OrganizationMemberCmd `cmd:"" help:"Manage organization members."`
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
		Name:  c.Name,
		Alias: c.Name,
	})
	if err != nil {
		// Check for unique constraint violation
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == pgerrcode.UniqueViolation {
			return fmt.Errorf("organization '%s' already exists", c.Name)
		}
		return fmt.Errorf("failed to create organization: %w", err)
	}

	ctx.Logger.Debug("organization created", "id", org.ID.String())

	return outputCreatedID(ctx.Output, org.ID)
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

// Run executes the organization delete command. The organization is
// soft-deleted: its memberships and API keys are revoked with it, and it is
// refused while clusters or published plugins still depend on it.
func (c *OrganizationDeleteCmd) Run(ctx *Context) error {
	ctx.Logger.Debug("deleting organization", "name", c.Name)

	bgCtx := context.Background()

	tx, err := ctx.DB.Pool.Begin(bgCtx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer rollback.Rollback(bgCtx, tx, ctx.Logger)

	qtx := ctx.Queries.WithTx(tx)

	orgID, err := qtx.OrganizationGetIDByNameForUpdate(bgCtx, db.OrganizationGetIDByNameForUpdateParams{Name: c.Name})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("organization '%s' not found", c.Name)
		}
		return fmt.Errorf("failed to look up organization: %w", err)
	}

	clusters, err := qtx.OrganizationCountLiveClusters(bgCtx, db.OrganizationCountLiveClustersParams{OrganizationID: orgID})
	if err != nil {
		return fmt.Errorf("failed to count clusters: %w", err)
	}
	if clusters > 0 {
		return fmt.Errorf("organization '%s' still has %d cluster(s); delete them first and wait until they are torn down", c.Name, clusters)
	}

	plugins, err := qtx.OrganizationCountLivePlugins(bgCtx, db.OrganizationCountLivePluginsParams{OrganizationID: orgID})
	if err != nil {
		return fmt.Errorf("failed to count plugins: %w", err)
	}
	if plugins > 0 {
		return fmt.Errorf("organization '%s' still publishes %d plugin(s); delete them first", c.Name, plugins)
	}

	revokedMemberships, err := qtx.MembershipRevokeAllForOrganization(bgCtx, db.MembershipRevokeAllForOrganizationParams{OrganizationID: orgID})
	if err != nil {
		return fmt.Errorf("failed to revoke memberships: %w", err)
	}

	revokedKeysCount, err := qtx.APIKeyRevokeAllForOrganization(bgCtx, db.APIKeyRevokeAllForOrganizationParams{OrganizationID: orgID})
	if err != nil {
		return fmt.Errorf("failed to revoke API keys: %w", err)
	}

	deleted, err := qtx.OrganizationDelete(bgCtx, db.OrganizationDeleteParams{ID: orgID})
	if err != nil {
		return fmt.Errorf("failed to delete organization: %w", err)
	}
	if deleted != 1 {
		return fmt.Errorf("organization '%s' not found", c.Name)
	}

	if err := tx.Commit(bgCtx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	ctx.Logger.Info("deleted organization", "name", c.Name, "id", orgID.String(),
		"memberships_revoked", revokedMemberships, "api_keys_revoked", revokedKeysCount)

	return nil
}

// organizationOutput is the JSON output structure for an organization.
type organizationOutput struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Alias   string `json:"alias"`
	Created string `json:"created"`
}

func outputOrganizationList(format OutputFormat, orgs []db.OrganizationListRow) error {
	switch format {
	case OutputJSON:
		output := make([]organizationOutput, len(orgs))
		for i, org := range orgs {
			output[i] = organizationOutput{
				ID:      org.ID.String(),
				Name:    org.Name,
				Alias:   org.Alias,
				Created: org.Created.Time.Format(TimeFormat),
			}
		}
		return PrintJSON(output)
	case OutputTable:
		w := NewTableWriter()
		fmt.Fprintln(w, "ID\tNAME\tALIAS\tCREATED")
		for _, org := range orgs {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				org.ID.String(),
				org.Name,
				org.Alias,
				org.Created.Time.Format(TimeFormat),
			)
		}
		return w.Flush()
	default:
		panic(fmt.Sprintf("unknown output format: %s", format))
	}
}
