package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/funops/pkg/db/gen"
)

// OrganizationQuotaCmd groups the quota commands. A quota caps what an
// organization may build: how many clusters it may have, and how many nodes of
// a machine type it may have in a region, summed over the autoscale maxima of
// its node pools. A new organization starts at one cluster and no machine
// quota at all, so nothing can be built in it until an operator sets quotas
// here. Lowering a quota below what is in use stops the next cluster or pool
// and leaves the existing ones alone.
type OrganizationQuotaCmd struct {
	GetClusters OrganizationQuotaGetClustersCmd `cmd:"" name:"get-clusters" help:"Show how many clusters an organization may have, and has."`
	SetClusters OrganizationQuotaSetClustersCmd `cmd:"" name:"set-clusters" help:"Set how many clusters an organization may have."`
	ListNodes   OrganizationQuotaListNodesCmd   `cmd:"" name:"list-nodes" help:"List an organization's node quotas per region and machine type, and what is in use."`
	SetNodes    OrganizationQuotaSetNodesCmd    `cmd:"" name:"set-nodes" help:"Set how many nodes of a machine type an organization may have in a region."`
}

// OrganizationQuotaGetClustersCmd shows the cluster quota next to the clusters
// in use, which may exceed a quota that was lowered.
type OrganizationQuotaGetClustersCmd struct {
	Organization string `arg:"" help:"Organization name." required:""`
}

// OrganizationQuotaListNodesCmd lists the node quotas next to what is in use,
// which may exceed a quota that was lowered.
type OrganizationQuotaListNodesCmd struct {
	Organization string `arg:"" help:"Organization name." required:""`
}

// OrganizationQuotaSetClustersCmd sets the cluster quota.
type OrganizationQuotaSetClustersCmd struct {
	Organization string `arg:"" help:"Organization name." required:""`
	Clusters     int32  `arg:"" help:"Maximum number of clusters, 0 or more. A deleted cluster counts until Gardener has torn it down." required:""`
}

// OrganizationQuotaSetNodesCmd sets the node quota of one machine type in one
// region.
type OrganizationQuotaSetNodesCmd struct {
	Organization string `arg:"" help:"Organization name." required:""`
	Region       string `arg:"" help:"Region name, as the catalog offers it." required:""`
	MachineType  string `arg:"" help:"Machine type name, as the catalog offers it in that region." required:""`
	MaxNodes     int32  `arg:"" help:"Maximum nodes of this machine type in this region, summed over the maxima of the organization's node pools; 0 takes the quota away." required:""`
}

// Run executes the organization quota get-clusters command.
func (c *OrganizationQuotaGetClustersCmd) Run(ctx *Context) error {
	ctx.Logger.Debug("reading cluster quota", "organization", c.Organization)

	bgCtx := context.Background()

	orgID, err := lookupOrganizationID(bgCtx, ctx.Queries, c.Organization)
	if err != nil {
		return err
	}

	clusters, err := ctx.Queries.QuotaClustersGet(bgCtx, db.QuotaClustersGetParams{ID: orgID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("organization %q not found", c.Organization)
		}
		return fmt.Errorf("failed to read cluster quota: %w", err)
	}

	return outputClusterQuota(ctx.Output, clusters)
}

// Run executes the organization quota list-nodes command.
func (c *OrganizationQuotaListNodesCmd) Run(ctx *Context) error {
	ctx.Logger.Debug("listing node quotas", "organization", c.Organization)

	bgCtx := context.Background()

	orgID, err := lookupOrganizationID(bgCtx, ctx.Queries, c.Organization)
	if err != nil {
		return err
	}

	machines, err := ctx.Queries.MachineQuotaList(bgCtx, db.MachineQuotaListParams{OrganizationID: orgID})
	if err != nil {
		return fmt.Errorf("failed to list node quotas: %w", err)
	}

	return outputNodeQuotaList(ctx.Output, machines)
}

// Run executes the organization quota set-clusters command.
func (c *OrganizationQuotaSetClustersCmd) Run(ctx *Context) error {
	if c.Clusters < 0 {
		return errors.New("the cluster quota must be 0 or more")
	}

	ctx.Logger.Debug("setting cluster quota", "organization", c.Organization, "clusters", c.Clusters)

	bgCtx := context.Background()

	orgID, err := lookupOrganizationID(bgCtx, ctx.Queries, c.Organization)
	if err != nil {
		return err
	}

	updated, err := ctx.Queries.QuotaClustersSet(bgCtx, db.QuotaClustersSetParams{ID: orgID, QuotaClusters: c.Clusters})
	if err != nil {
		return fmt.Errorf("failed to set cluster quota: %w", err)
	}
	if updated != 1 {
		return fmt.Errorf("organization %q not found", c.Organization)
	}

	ctx.Logger.Info("cluster quota set", "organization", c.Organization, "clusters", c.Clusters)

	return nil
}

// Run executes the organization quota set-nodes command. The offering is
// resolved against the catalog first, so a mistyped region or machine type is
// an error rather than a quota nothing can use.
func (c *OrganizationQuotaSetNodesCmd) Run(ctx *Context) error {
	if c.MaxNodes < 0 {
		return errors.New("the node quota must be 0 or more")
	}

	ctx.Logger.Debug("setting node quota", "organization", c.Organization,
		"region", c.Region, "machine_type", c.MachineType, "max_nodes", c.MaxNodes)

	bgCtx := context.Background()

	orgID, err := lookupOrganizationID(bgCtx, ctx.Queries, c.Organization)
	if err != nil {
		return err
	}

	offering, err := ctx.Queries.RegionMachineTypeResolve(bgCtx, db.RegionMachineTypeResolveParams{
		RegionName:      c.Region,
		MachineTypeName: c.MachineType,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("machine type %q is not offered in region %q", c.MachineType, c.Region)
		}
		return fmt.Errorf("failed to resolve machine type: %w", err)
	}

	written, err := ctx.Queries.MachineQuotaSet(bgCtx, db.MachineQuotaSetParams{
		OrganizationID:      orgID,
		RegionMachineTypeID: offering,
		MaxNodes:            c.MaxNodes,
	})
	if err != nil {
		return fmt.Errorf("failed to set node quota: %w", err)
	}
	if written != 1 {
		return fmt.Errorf("organization %q not found", c.Organization)
	}

	ctx.Logger.Info("node quota set", "organization", c.Organization,
		"region", c.Region, "machine_type", c.MachineType, "max_nodes", c.MaxNodes)

	return nil
}

// clusterQuotaOutput is the JSON output structure of quota get-clusters.
type clusterQuotaOutput struct {
	Quota int32 `json:"quota"`
	InUse int64 `json:"in_use"`
}

// nodeQuotaOutput is one row of quota list-nodes in JSON.
type nodeQuotaOutput struct {
	Region      string `json:"region"`
	MachineType string `json:"machine_type"`
	Quota       int32  `json:"quota"`
	InUse       int64  `json:"in_use"`
	// Empty for an offering in use without a quota row, which only pools from
	// before quotas existed can be.
	Updated string `json:"updated"`
}

// quotaUpdated formats when a quota row was last set; a row-less offering in
// use shows "-" in the table and "" in JSON.
func quotaUpdated(updated pgtype.Timestamptz) string {
	if !updated.Valid {
		return ""
	}
	return updated.Time.Format(TimeFormat)
}

func outputClusterQuota(format OutputFormat, clusters db.QuotaClustersGetRow) error {
	switch format {
	case OutputJSON:
		return PrintJSON(clusterQuotaOutput{Quota: clusters.QuotaClusters, InUse: clusters.ClustersInUse})
	case OutputTable:
		w := NewTableWriter()
		if _, err := fmt.Fprintln(w, "QUOTA\tIN_USE"); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
		if _, err := fmt.Fprintf(w, "%d\t%d\n", clusters.QuotaClusters, clusters.ClustersInUse); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
		if err := w.Flush(); err != nil {
			return fmt.Errorf("flushing output: %w", err)
		}
		return nil
	default:
		panic(fmt.Sprintf("unknown output format: %s", format))
	}
}

func outputNodeQuotaList(format OutputFormat, machines []db.MachineQuotaListRow) error {
	switch format {
	case OutputJSON:
		output := make([]nodeQuotaOutput, len(machines))
		for i := range machines {
			m := &machines[i]
			output[i] = nodeQuotaOutput{
				Region:      m.Region,
				MachineType: m.MachineType,
				Quota:       m.MaxNodes,
				InUse:       m.NodesInUse,
				Updated:     quotaUpdated(m.Updated),
			}
		}
		return PrintJSON(output)
	case OutputTable:
		w := NewTableWriter()
		if _, err := fmt.Fprintln(w, "REGION\tMACHINE_TYPE\tQUOTA\tIN_USE\tUPDATED"); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
		for i := range machines {
			m := &machines[i]
			updated := quotaUpdated(m.Updated)
			if updated == "" {
				updated = "-"
			}
			if _, err := fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\n",
				m.Region,
				m.MachineType,
				m.MaxNodes,
				m.NodesInUse,
				updated,
			); err != nil {
				return fmt.Errorf("writing output: %w", err)
			}
		}
		if err := w.Flush(); err != nil {
			return fmt.Errorf("flushing output: %w", err)
		}
		return nil
	default:
		panic(fmt.Sprintf("unknown output format: %s", format))
	}
}
