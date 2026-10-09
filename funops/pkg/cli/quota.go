package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

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
	List        OrganizationQuotaListCmd        `cmd:"" help:"Show an organization's quotas and what is in use."`
	SetClusters OrganizationQuotaSetClustersCmd `cmd:"" name:"set-clusters" help:"Set how many clusters an organization may have."`
	SetNodes    OrganizationQuotaSetNodesCmd    `cmd:"" name:"set-nodes" help:"Set how many nodes of a machine type an organization may have in a region."`
}

// OrganizationQuotaListCmd shows an organization's quotas next to what is in
// use, which may exceed a quota that was lowered.
type OrganizationQuotaListCmd struct {
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

// Run executes the organization quota list command.
func (c *OrganizationQuotaListCmd) Run(ctx *Context) error {
	ctx.Logger.Debug("listing quotas", "organization", c.Organization)

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

	machines, err := ctx.Queries.MachineQuotaList(bgCtx, db.MachineQuotaListParams{OrganizationID: orgID})
	if err != nil {
		return fmt.Errorf("failed to list machine quotas: %w", err)
	}

	return outputQuotaList(ctx.Output, clusters, machines)
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

	err = ctx.Queries.MachineQuotaSet(bgCtx, db.MachineQuotaSetParams{
		OrganizationID:      orgID,
		RegionMachineTypeID: offering,
		MaxNodes:            c.MaxNodes,
	})
	if err != nil {
		return fmt.Errorf("failed to set node quota: %w", err)
	}

	ctx.Logger.Info("node quota set", "organization", c.Organization,
		"region", c.Region, "machine_type", c.MachineType, "max_nodes", c.MaxNodes)

	return nil
}

// quotaOutput is the JSON output structure of the quota list.
type quotaOutput struct {
	Clusters clusterQuotaOutput `json:"clusters"`
	Nodes    []nodeQuotaOutput  `json:"nodes"`
}

type clusterQuotaOutput struct {
	Quota int32 `json:"quota"`
	InUse int64 `json:"in_use"`
}

type nodeQuotaOutput struct {
	Region      string `json:"region"`
	MachineType string `json:"machine_type"`
	Quota       int32  `json:"quota"`
	InUse       int64  `json:"in_use"`
	Updated     string `json:"updated"`
}

func outputQuotaList(format OutputFormat, clusters db.QuotaClustersGetRow, machines []db.MachineQuotaListRow) error {
	switch format {
	case OutputJSON:
		output := quotaOutput{
			Clusters: clusterQuotaOutput{Quota: clusters.QuotaClusters, InUse: clusters.ClustersInUse},
			Nodes:    make([]nodeQuotaOutput, len(machines)),
		}
		for i := range machines {
			m := &machines[i]
			output.Nodes[i] = nodeQuotaOutput{
				Region:      m.Region,
				MachineType: m.MachineType,
				Quota:       m.MaxNodes,
				InUse:       m.NodesInUse,
				Updated:     m.Updated.Time.Format(TimeFormat),
			}
		}
		return PrintJSON(output)
	case OutputTable:
		w := NewTableWriter()
		if _, err := fmt.Fprintln(w, "KIND\tREGION\tMACHINE_TYPE\tQUOTA\tIN_USE\tUPDATED"); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
		if _, err := fmt.Fprintf(w, "clusters\t-\t-\t%d\t%d\t-\n", clusters.QuotaClusters, clusters.ClustersInUse); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
		for i := range machines {
			m := &machines[i]
			if _, err := fmt.Fprintf(w, "nodes\t%s\t%s\t%d\t%d\t%s\n",
				m.Region,
				m.MachineType,
				m.MaxNodes,
				m.NodesInUse,
				m.Updated.Time.Format(TimeFormat),
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
