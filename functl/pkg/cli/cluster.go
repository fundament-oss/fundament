package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	db "github.com/fundament-oss/fundament/functl/pkg/db/gen"
)

type ClusterCmd struct {
	Create ClusterCreateCmd `cmd:"" help:"Create a new cluster."`
	Get    ClusterGetCmd    `cmd:"" help:"Get cluster details."`
	List   ClusterListCmd   `cmd:"" help:"List clusters in an organization."`
	Update ClusterUpdateCmd `cmd:"" help:"Update a cluster."`
	Delete ClusterDeleteCmd `cmd:"" help:"Delete a cluster."`
}

type ClusterCreateCmd struct {
	Identifier        string `arg:"" help:"Cluster identifier: <organization>/<cluster>." required:""`
	Region            string `help:"Cloud region for the cluster." required:""`
	KubernetesVersion string `help:"Kubernetes version." required:"" name:"kubernetes-version"`
}

type ClusterGetCmd struct {
	Identifier string `arg:"" help:"Cluster identifier: <organization>/<cluster>." required:""`
}

type ClusterListCmd struct {
	Organization string `arg:"" help:"Organization name." required:""`
}

type ClusterUpdateCmd struct {
	Identifier        string `arg:"" help:"Cluster identifier: <organization>/<cluster>." required:""`
	KubernetesVersion string `help:"New Kubernetes version." required:"" name:"kubernetes-version"`
}

type ClusterDeleteCmd struct {
	Identifier string `arg:"" help:"Cluster identifier: <organization>/<cluster>." required:""`
}

// parseClusterIdentifier splits "<organization>/<cluster>" into its parts.
func parseClusterIdentifier(identifier string) (organization, cluster string, err error) {
	parts := strings.SplitN(identifier, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid cluster identifier '%s': expected format <organization>/<cluster>", identifier)
	}
	return parts[0], parts[1], nil
}

// Run executes the cluster create command.
func (c *ClusterCreateCmd) Run(ctx *Context) error {
	org, cluster, err := parseClusterIdentifier(c.Identifier)
	if err != nil {
		return err
	}

	ctx.Logger.Debug("creating cluster", "organization", org, "cluster", cluster, "region", c.Region, "kubernetes_version", c.KubernetesVersion)

	cl, err := ctx.Queries.ClusterCreate(context.Background(), db.ClusterCreateParams{
		OrganizationName:  org,
		Name:              cluster,
		Region:            c.Region,
		KubernetesVersion: c.KubernetesVersion,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("organization '%s' not found", org)
		}
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == pgerrcode.UniqueViolation {
			return fmt.Errorf("cluster '%s' already exists in organization '%s'", cluster, org)
		}
		return fmt.Errorf("failed to create cluster: %w", err)
	}

	ctx.Logger.Debug("cluster created", "id", cl.ID.String())

	return outputClusterCreate(ctx.Output, cl)
}

// Run executes the cluster get command.
func (c *ClusterGetCmd) Run(ctx *Context) error {
	org, cluster, err := parseClusterIdentifier(c.Identifier)
	if err != nil {
		return err
	}

	ctx.Logger.Debug("getting cluster", "organization", org, "cluster", cluster)

	cl, err := ctx.Queries.ClusterGet(context.Background(), db.ClusterGetParams{
		OrganizationName: org,
		ClusterName:      cluster,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("cluster '%s' not found", c.Identifier)
		}
		return fmt.Errorf("failed to get cluster: %w", err)
	}

	ctx.Logger.Debug("cluster retrieved", "id", cl.ID.String())

	return outputClusterGet(ctx.Output, org, cl)
}

// Run executes the cluster list command.
func (c *ClusterListCmd) Run(ctx *Context) error {
	ctx.Logger.Debug("listing clusters", "organization", c.Organization)

	clusters, err := ctx.Queries.ClusterList(context.Background(), db.ClusterListParams{
		OrganizationName: c.Organization,
	})
	if err != nil {
		return fmt.Errorf("failed to list clusters: %w", err)
	}

	ctx.Logger.Debug("clusters listed", "count", len(clusters))

	return outputClusterList(ctx.Output, c.Organization, clusters)
}

// Run executes the cluster update command.
func (c *ClusterUpdateCmd) Run(ctx *Context) error {
	org, cluster, err := parseClusterIdentifier(c.Identifier)
	if err != nil {
		return err
	}

	ctx.Logger.Debug("updating cluster", "organization", org, "cluster", cluster, "kubernetes_version", c.KubernetesVersion)

	cl, err := ctx.Queries.ClusterUpdate(context.Background(), db.ClusterUpdateParams{
		OrganizationName:  org,
		ClusterName:       cluster,
		KubernetesVersion: c.KubernetesVersion,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("cluster '%s' not found", c.Identifier)
		}
		return fmt.Errorf("failed to update cluster: %w", err)
	}

	ctx.Logger.Debug("cluster updated", "id", cl.ID.String())

	return outputClusterUpdate(ctx.Output, cl)
}

// Run executes the cluster delete command.
func (c *ClusterDeleteCmd) Run(ctx *Context) error {
	org, cluster, err := parseClusterIdentifier(c.Identifier)
	if err != nil {
		return err
	}

	ctx.Logger.Debug("deleting cluster", "organization", org, "cluster", cluster)

	rowsAffected, err := ctx.Queries.ClusterDelete(context.Background(), db.ClusterDeleteParams{
		OrganizationName: org,
		ClusterName:      cluster,
	})
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == pgerrcode.RaiseException {
			return fmt.Errorf("cannot delete cluster '%s': cluster has undeleted namespaces", c.Identifier)
		}
		return fmt.Errorf("failed to delete cluster: %w", err)
	}
	if rowsAffected != 1 {
		return fmt.Errorf("cluster '%s' not found", c.Identifier)
	}

	ctx.Logger.Info("deleted cluster", "identifier", c.Identifier)

	return nil
}

// clusterCreateOutput is the JSON output structure for cluster create.
type clusterCreateOutput struct {
	ID string `json:"id"`
}

// clusterOutput is the JSON output structure for a cluster.
type clusterOutput struct {
	ID                string `json:"id"`
	Identifier        string `json:"identifier"`
	Region            string `json:"region"`
	KubernetesVersion string `json:"kubernetes_version"`
	Status            string `json:"status"`
	Created           string `json:"created"`
}

// clusterDetailOutput is the JSON output structure for cluster get.
type clusterDetailOutput struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Region            string `json:"region"`
	KubernetesVersion string `json:"kubernetes_version"`
	Status            string `json:"status"`
	Created           string `json:"created"`
}

func outputClusterCreate(format OutputFormat, cl db.ClusterCreateRow) error {
	switch format {
	case OutputJSON:
		return PrintJSON(clusterCreateOutput{
			ID: cl.ID.String(),
		})
	case OutputTable:
		fmt.Println(cl.ID.String())
		return nil
	default:
		panic(fmt.Sprintf("unknown output format: %s", format))
	}
}

func outputClusterGet(format OutputFormat, organization string, cl db.ClusterGetRow) error {
	switch format {
	case OutputJSON:
		return PrintJSON(clusterDetailOutput{
			ID:                cl.ID.String(),
			Name:              cl.Name,
			Region:            cl.Region,
			KubernetesVersion: cl.KubernetesVersion,
			Status:            cl.Status,
			Created:           cl.Created.Time.Format(TimeFormat),
		})
	case OutputTable:
		w := NewTableWriter()
		fmt.Fprintf(w, "ID:\t%s\n", cl.ID.String())
		fmt.Fprintf(w, "Name:\t%s\n", cl.Name)
		fmt.Fprintf(w, "Organization:\t%s\n", organization)
		fmt.Fprintf(w, "Region:\t%s\n", cl.Region)
		fmt.Fprintf(w, "Kubernetes Version:\t%s\n", cl.KubernetesVersion)
		fmt.Fprintf(w, "Status:\t%s\n", cl.Status)
		fmt.Fprintf(w, "Created:\t%s\n", cl.Created.Time.Format(TimeFormat))
		return w.Flush()
	default:
		panic(fmt.Sprintf("unknown output format: %s", format))
	}
}

func outputClusterList(format OutputFormat, organization string, clusters []db.ClusterListRow) error {
	switch format {
	case OutputJSON:
		output := make([]clusterOutput, len(clusters))
		for i, cl := range clusters {
			output[i] = clusterOutput{
				ID:                cl.ID.String(),
				Identifier:        organization + "/" + cl.Name,
				Region:            cl.Region,
				KubernetesVersion: cl.KubernetesVersion,
				Status:            cl.Status,
				Created:           cl.Created.Time.Format(TimeFormat),
			}
		}
		return PrintJSON(output)
	case OutputTable:
		w := NewTableWriter()
		fmt.Fprintln(w, "ID\tIDENTIFIER\tREGION\tK8S VERSION\tSTATUS\tCREATED")
		for _, cl := range clusters {
			fmt.Fprintf(w, "%s\t%s/%s\t%s\t%s\t%s\t%s\n",
				cl.ID.String(),
				organization,
				cl.Name,
				cl.Region,
				cl.KubernetesVersion,
				cl.Status,
				cl.Created.Time.Format(TimeFormat),
			)
		}
		return w.Flush()
	default:
		panic(fmt.Sprintf("unknown output format: %s", format))
	}
}

func outputClusterUpdate(format OutputFormat, cl db.ClusterUpdateRow) error {
	switch format {
	case OutputJSON:
		return PrintJSON(clusterCreateOutput{
			ID: cl.ID.String(),
		})
	case OutputTable:
		fmt.Println(cl.ID.String())
		return nil
	default:
		panic(fmt.Sprintf("unknown output format: %s", format))
	}
}
