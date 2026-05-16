package provider

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// waitForClusterRunning polls GetCluster until status == RUNNING.
// Returns error if cluster enters ERROR state or ctx deadline elapses.
func waitForClusterRunning(ctx context.Context, client *FundamentClient, clusterID string) error {
	for {
		req := connect.NewRequest(organizationv1.GetClusterRequest_builder{
			ClusterId: clusterID,
		}.Build())

		resp, err := client.ClusterService.GetCluster(ctx, req)
		if err != nil {
			return fmt.Errorf("polling cluster status: %w", err)
		}

		switch resp.Msg.GetCluster().GetStatus() {
		case organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING:
			return nil
		case organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR:
			return fmt.Errorf("cluster %s entered ERROR state", clusterID)
		case organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING,
			organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPING,
			organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPED:
			return fmt.Errorf("cluster %s is in a terminal state and will not reach RUNNING", clusterID)
		default:
		}

		t := time.NewTimer(10 * time.Second)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}
