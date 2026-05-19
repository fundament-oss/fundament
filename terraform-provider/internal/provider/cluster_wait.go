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
// Transient errors (including not_found, which can occur briefly after creation)
// are retried; only consecutive failures beyond the threshold are fatal.
func waitForClusterRunning(ctx context.Context, client *FundamentClient, clusterID string) error {
	const maxConsecutiveErrors = 5

	consecutiveErrors := 0
	lastStatus := organizationv1.ClusterStatus_CLUSTER_STATUS_UNSPECIFIED

	for {
		req := connect.NewRequest(organizationv1.GetClusterRequest_builder{
			ClusterId: clusterID,
		}.Build())

		resp, err := client.ClusterService.GetCluster(ctx, req)
		if err != nil {
			consecutiveErrors++
			if consecutiveErrors >= maxConsecutiveErrors {
				return fmt.Errorf("polling cluster status: %w", err)
			}
			t := time.NewTimer(10 * time.Second)
			select {
			case <-ctx.Done():
				t.Stop()
				return fmt.Errorf("timed out waiting for cluster %s to reach RUNNING (last status: %s): %w", clusterID, lastStatus, ctx.Err())
			case <-t.C:
			}
			continue
		}

		consecutiveErrors = 0
		lastStatus = resp.Msg.GetCluster().GetStatus()

		switch lastStatus {
		case organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING:
			return nil
		case organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR:
			return fmt.Errorf("cluster %s entered ERROR state", clusterID)
		case organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING,
			organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPING,
			organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPED:
			return fmt.Errorf("cluster %s is in a terminal state and will not reach RUNNING", clusterID)
		default:
			// CREATING, UPGRADING, UNSPECIFIED, and any future transient states — keep polling.
		}

		t := time.NewTimer(10 * time.Second)
		select {
		case <-ctx.Done():
			t.Stop()
			return fmt.Errorf("timed out waiting for cluster %s to reach RUNNING (last status: %s): %w", clusterID, lastStatus, ctx.Err())
		case <-t.C:
		}
	}
}
