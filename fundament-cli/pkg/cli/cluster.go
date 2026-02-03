package cli

import (
	"context"
	"fmt"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// ClusterCmd contains cluster subcommands.
type ClusterCmd struct {
	List ClusterListCmd `cmd:"" help:"List all clusters."`
	Get  ClusterGetCmd  `cmd:"" help:"Get cluster details."`
}

// ClusterListCmd handles the cluster list command.
type ClusterListCmd struct{}

// Run executes the cluster list command.
func (c *ClusterListCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig()
	if err != nil {
		return err
	}

	clusters, err := apiClient.ListClusters(context.Background())
	if err != nil {
		return fmt.Errorf("failed to list clusters: %w", err)
	}

	if ctx.Output == OutputJSON {
		return PrintJSON(clusters)
	}

	if len(clusters) == 0 {
		fmt.Println("No clusters found")
		return nil
	}

	w := NewTableWriter()
	fmt.Fprintln(w, "ID\tNAME\tSTATUS\tREGION")
	for _, cluster := range clusters {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			cluster.Id,
			cluster.Name,
			formatClusterStatus(cluster.Status),
			cluster.Region,
		)
	}
	return w.Flush()
}

// ClusterGetCmd handles the cluster get command.
type ClusterGetCmd struct {
	ClusterID string `arg:"" help:"Cluster ID to get."`
}

// Run executes the cluster get command.
func (c *ClusterGetCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig()
	if err != nil {
		return err
	}

	cluster, err := apiClient.GetCluster(context.Background(), c.ClusterID)
	if err != nil {
		return fmt.Errorf("failed to get cluster: %w", err)
	}

	if ctx.Output == OutputJSON {
		return PrintJSON(cluster)
	}

	w := NewTableWriter()
	PrintKeyValue(w, "ID", cluster.Id)
	PrintKeyValue(w, "Name", cluster.Name)
	PrintKeyValue(w, "Region", cluster.Region)
	PrintKeyValue(w, "Kubernetes Version", cluster.KubernetesVersion)
	PrintKeyValue(w, "Status", formatClusterStatus(cluster.Status))
	if cluster.CreatedAt != nil {
		PrintKeyValue(w, "Created", cluster.CreatedAt.AsTime().Format(TimeFormat))
	}
	return w.Flush()
}

// formatClusterStatus formats a cluster status for display.
func formatClusterStatus(status organizationv1.ClusterStatus) string {
	switch status {
	case organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING:
		return "provisioning"
	case organizationv1.ClusterStatus_CLUSTER_STATUS_STARTING:
		return "starting"
	case organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING:
		return "running"
	case organizationv1.ClusterStatus_CLUSTER_STATUS_UPGRADING:
		return "upgrading"
	case organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR:
		return "error"
	case organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPING:
		return "stopping"
	case organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPED:
		return "stopped"
	default:
		return "unknown"
	}
}
