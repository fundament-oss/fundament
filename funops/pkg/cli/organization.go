package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

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
	Quota  OrganizationQuotaCmd  `cmd:"" help:"Manage what an organization may build: its cluster and node quotas."`
}

// OrganizationCreateCmd creates a new organization with its starting node
// quotas. A cluster runs only once it has a node pool, and a pool needs a node
// quota, so an organization is never created without one.
type OrganizationCreateCmd struct {
	Name      string   `arg:"" help:"Organization name." required:""`
	NodeQuota []string `name:"node-quota" required:"" placeholder:"REGION/MACHINE-TYPE=MAX" help:"Starting node quota: at most MAX nodes of MACHINE-TYPE in REGION, as the catalog offers them, for example local/local-small=3. Repeat it for more machine types; change it later with 'quota set-nodes'."`
}

// nodeQuotaSpec is one parsed --node-quota value.
type nodeQuotaSpec struct {
	Region      string
	MachineType string
	MaxNodes    int32
}

// parseNodeQuota parses REGION/MACHINE-TYPE=MAX. A starting quota of 0 would
// allow no pool at all, so MAX must be 1 or more.
func parseNodeQuota(value string) (nodeQuotaSpec, error) {
	offering, maxText, ok := strings.Cut(value, "=")
	region, machineType, ok2 := strings.Cut(offering, "/")
	if !ok || !ok2 || region == "" || machineType == "" || maxText == "" {
		return nodeQuotaSpec{}, fmt.Errorf("node quota %q is not REGION/MACHINE-TYPE=MAX", value)
	}
	maxNodes, err := strconv.ParseInt(maxText, 10, 32)
	if err != nil || maxNodes < 1 {
		return nodeQuotaSpec{}, fmt.Errorf("node quota %q: MAX must be a whole number of nodes, 1 or more", value)
	}
	return nodeQuotaSpec{Region: region, MachineType: machineType, MaxNodes: int32(maxNodes)}, nil
}

// OrganizationListCmd lists all organizations.
type OrganizationListCmd struct{}

// OrganizationDeleteCmd deletes an organization.
type OrganizationDeleteCmd struct {
	Name string `arg:"" help:"Organization name." required:""`
}

// Run executes the organization create command.
func (c *OrganizationCreateCmd) Run(ctx *Context) error {
	if len(c.NodeQuota) == 0 {
		return errors.New("an organization needs a starting node quota: give at least one --node-quota REGION/MACHINE-TYPE=MAX")
	}
	quotas := make([]nodeQuotaSpec, 0, len(c.NodeQuota))
	seen := make(map[string]bool, len(c.NodeQuota))
	for _, value := range c.NodeQuota {
		quota, err := parseNodeQuota(value)
		if err != nil {
			return err
		}
		// A second value for the same offering would silently win.
		offering := quota.Region + "/" + quota.MachineType
		if seen[offering] {
			return fmt.Errorf("node quota for %s is given more than once", offering)
		}
		seen[offering] = true
		quotas = append(quotas, quota)
	}

	ctx.Logger.Debug("creating organization", "name", c.Name)

	bgCtx := context.Background()

	tx, err := ctx.DB.Pool.Begin(bgCtx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer rollback.Rollback(bgCtx, tx, ctx.Logger)

	qtx := ctx.Queries.WithTx(tx)

	org, err := qtx.OrganizationCreate(bgCtx, db.OrganizationCreateParams{
		Name:  c.Name,
		Alias: c.Name,
	})
	if err != nil {
		// Check for unique constraint violation
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == pgerrcode.UniqueViolation {
			return fmt.Errorf("organization '%s' already exists", c.Name)
		}
		// No row: a deleted organization that had clusters still holds the
		// name, because its Gardener project is named after it.
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("organization name '%s' is still held by a deleted organization that had clusters; its Gardener project is not cleaned up yet", c.Name)
		}
		return fmt.Errorf("failed to create organization: %w", err)
	}

	// Resolved in the same transaction: a mistyped offering creates nothing.
	for _, quota := range quotas {
		offering, err := qtx.RegionMachineTypeResolve(bgCtx, db.RegionMachineTypeResolveParams{
			RegionName:      quota.Region,
			MachineTypeName: quota.MachineType,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("machine type %q is not offered in region %q", quota.MachineType, quota.Region)
			}
			return fmt.Errorf("failed to resolve machine type: %w", err)
		}
		if _, err := qtx.MachineQuotaSet(bgCtx, db.MachineQuotaSetParams{
			OrganizationID:      org.ID,
			RegionMachineTypeID: offering,
			MaxNodes:            quota.MaxNodes,
		}); err != nil {
			return fmt.Errorf("failed to set node quota: %w", err)
		}
	}

	if err := tx.Commit(bgCtx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	ctx.Logger.Debug("organization created", "id", org.ID.String(), "node_quotas", len(quotas))

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
