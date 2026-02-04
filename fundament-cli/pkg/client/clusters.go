package client

import (
	"context"

	"connectrpc.com/connect"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// ListClusters lists all clusters.
func (c *Client) ListClusters(ctx context.Context) ([]*organizationv1.ClusterSummary, error) {
	resp, err := c.clusters().ListClusters(ctx, connect.NewRequest(&organizationv1.ListClustersRequest{}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Clusters, nil
}

// GetCluster gets a cluster by ID.
func (c *Client) GetCluster(ctx context.Context, clusterID string) (*organizationv1.ClusterDetails, error) {
	resp, err := c.clusters().GetCluster(ctx, connect.NewRequest(&organizationv1.GetClusterRequest{
		ClusterId: clusterID,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Cluster, nil
}
