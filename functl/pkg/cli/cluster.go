package cli

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

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

	resp, err := apiClient.Clusters().ListClusters(context.Background(), connect.NewRequest(organizationv1.ListClustersRequest_builder{}.Build()))
	if err != nil {
		return fmt.Errorf("failed to list clusters: %w", err)
	}

	clusters := resp.Msg.GetClusters()

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
			cluster.GetId(),
			cluster.GetName(),
			formatClusterStatus(cluster.GetStatus()),
			cluster.GetRegion(),
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

	resp, err := apiClient.Clusters().GetCluster(context.Background(), connect.NewRequest(organizationv1.GetClusterRequest_builder{
		ClusterId: c.ClusterID,
	}.Build()))
	if err != nil {
		return fmt.Errorf("failed to get cluster: %w", err)
	}

	cluster := resp.Msg.GetCluster()

	if ctx.Output == OutputJSON {
		return PrintJSON(cluster)
	}

	w := NewTableWriter()
	PrintKeyValue(w, "ID", cluster.GetId())
	PrintKeyValue(w, "Name", cluster.GetName())
	PrintKeyValue(w, "Region", cluster.GetRegion())
	PrintKeyValue(w, "Kubernetes Version", cluster.GetKubernetesVersion())
	PrintKeyValue(w, "Status", formatClusterStatus(cluster.GetStatus()))
	if cluster.GetCreated() != nil {
		PrintKeyValue(w, "Created", cluster.GetCreated().AsTime().Format(TimeFormat))
	}
	return w.Flush()
}

// formatClusterStatus formats a cluster status for display.
func formatClusterStatus(status organizationv1.ClusterStatus) string {
	switch status {
	case organizationv1.ClusterStatus_CLUSTER_STATUS_UNSPECIFIED:
		return "unspecified"
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
