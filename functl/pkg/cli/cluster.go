package cli

import (
	"context"
	"fmt"
	"os"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// ClusterCmd contains cluster subcommands.
type ClusterCmd struct {
	List       ClusterListCmd       `cmd:"" help:"List all clusters."`
	Get        ClusterGetCmd        `cmd:"" help:"Get cluster details."`
	Kubeconfig ClusterKubeconfigCmd `cmd:"" help:"Generate kubeconfig for a cluster."`
	Token      ClusterTokenCmd      `cmd:"" help:"Print a short-lived platform token as a kubectl ExecCredential (used by the kubeconfig from 'cluster kubeconfig')."`
}

// ClusterListCmd handles the cluster list command.
type ClusterListCmd struct{}

// Run executes the cluster list command.
func (c *ClusterListCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfigWithOrg(ctx)
	if err != nil {
		return err
	}

	resp, err := apiClient.Clusters().ListClusters(context.Background(), organizationv1.ListClustersRequest_builder{}.Build())
	if err != nil {
		return fmt.Errorf("failed to list clusters: %w", err)
	}

	clusters := resp.GetClusters()

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
	apiClient, err := NewClientFromConfigWithOrg(ctx)
	if err != nil {
		return err
	}

	resp, err := apiClient.Clusters().GetCluster(context.Background(), organizationv1.GetClusterRequest_builder{
		ClusterId: c.ClusterID,
	}.Build())
	if err != nil {
		return fmt.Errorf("failed to get cluster: %w", err)
	}

	cluster := resp.GetCluster()

	// The pool list is auxiliary: when it fails, the cluster — what the
	// operator came for — still prints, with a warning on stderr.
	var nodePools []*organizationv1.NodePool
	npResp, npErr := apiClient.Clusters().ListNodePools(context.Background(), organizationv1.ListNodePoolsRequest_builder{
		ClusterId: c.ClusterID,
	}.Build())
	if npErr != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to list node pools: %v\n", npErr)
	} else {
		nodePools = npResp.GetNodePools()
	}

	if ctx.Output == OutputJSON {
		// node_pools is null when the lookup failed, [] when there are none.
		var pools any
		if npErr == nil {
			pools = nodePools
		}
		return PrintJSON(map[string]any{"cluster": cluster, "node_pools": pools})
	}

	w := NewTableWriter()
	PrintKeyValue(w, "ID", cluster.GetId())
	PrintKeyValue(w, "Name", cluster.GetName())
	PrintKeyValue(w, "Region", cluster.GetRegion())
	PrintKeyValue(w, "Kubernetes Version", cluster.GetKubernetesVersion())
	PrintKeyValue(w, "Status", formatClusterStatus(cluster.GetStatus()))
	if sync := cluster.GetSyncState(); sync != nil {
		if sync.HasOutboxStatus() {
			PrintKeyValue(w, "Sync Status", sync.GetOutboxStatus())
		}
		if sync.HasOutboxMessage() {
			PrintKeyValue(w, "Sync Message", sync.GetOutboxMessage())
		}
		if sync.HasOutboxError() {
			PrintKeyValue(w, "Sync Error", sync.GetOutboxError())
		}
	}
	if cluster.GetCreated() != nil {
		PrintKeyValue(w, "Created", cluster.GetCreated().AsTime().Format(TimeFormat))
	}
	if len(nodePools) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "NAME\tTYPE\tNODES\tSTATUS\tREASON\tSYNC ERROR")
		for _, pool := range nodePools {
			fmt.Fprintf(w, "%s\t%s\t%d (min %d, max %d)\t%s\t%s\t%s\n",
				pool.GetName(),
				pool.GetMachineType(),
				pool.GetCurrentNodes(), pool.GetMinNodes(), pool.GetMaxNodes(),
				formatNodePoolStatus(pool.GetStatus()),
				pool.GetStatusMessage(),
				pool.GetSyncError(),
			)
		}
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
		return "updating"
	case organizationv1.ClusterStatus_CLUSTER_STATUS_UNHEALTHY:
		return "unhealthy"
	case organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR:
		return "error"
	case organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPING:
		return "stopping"
	case organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPED:
		return "stopped"
	case organizationv1.ClusterStatus_CLUSTER_STATUS_LOST:
		return "lost"
	default:
		return "unknown"
	}
}

// formatNodePoolStatus formats a node pool status for display.
func formatNodePoolStatus(status organizationv1.NodePoolStatus) string {
	switch status {
	case organizationv1.NodePoolStatus_NODE_POOL_STATUS_HEALTHY:
		return "healthy"
	case organizationv1.NodePoolStatus_NODE_POOL_STATUS_DEGRADED:
		return "degraded"
	case organizationv1.NodePoolStatus_NODE_POOL_STATUS_UNHEALTHY:
		return "unhealthy"
	case organizationv1.NodePoolStatus_NODE_POOL_STATUS_PROVISIONING:
		return "provisioning"
	case organizationv1.NodePoolStatus_NODE_POOL_STATUS_WAITING_FOR_MACHINES:
		return "waiting for machines"
	case organizationv1.NodePoolStatus_NODE_POOL_STATUS_FAILED:
		return "failed"
	case organizationv1.NodePoolStatus_NODE_POOL_STATUS_UNSPECIFIED:
		return "unknown"
	default:
		return "unknown"
	}
}
