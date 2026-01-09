package adapter

import (
	"fmt"
	"time"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/models"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/google/uuid"
)

func ToClusterCreate(req *organizationv1.CreateClusterRequest) models.ClusterCreate {
	nodePools := make([]models.NodePoolCreate, 0, len(req.NodePools))

	for _, nodePool := range req.NodePools {
		nodePools = append(nodePools, ToNodePoolCreate(nodePool))
	}

	return models.ClusterCreate{
		Name:              req.Name,
		Region:            req.Region,
		KubernetesVersion: req.KubernetesVersion,
		NodePools:         nodePools,
	}
}

func ToClusterUpdate(req *organizationv1.UpdateClusterRequest) (models.ClusterUpdate, error) {
	clusterID, err := uuid.Parse(req.ClusterId)
	if err != nil {
		return models.ClusterUpdate{}, fmt.Errorf("cluster id parse: %w", err)
	}

	return models.ClusterUpdate{
		ClusterID:         clusterID,
		KubernetesVersion: req.KubernetesVersion,
	}, nil
}

func FromClustersSummary(clusters []db.TenantCluster) []*organizationv1.ClusterSummary {
	summaries := make([]*organizationv1.ClusterSummary, 0, len(clusters))
	for _, c := range clusters {
		summaries = append(summaries, FromClusterSummary(c))
	}
	return summaries
}

func FromClusterSummary(c db.TenantCluster) *organizationv1.ClusterSummary {
	return &organizationv1.ClusterSummary{
		Id:            c.ID.String(),
		Name:          c.Name,
		Status:        FromClusterStatus(c.Status),
		Region:        c.Region,
		ProjectCount:  0, // Stub
		NodePoolCount: c.NodePoolCount,
	}
}

func FromClusterDetail(c db.TenantCluster) *organizationv1.ClusterDetails {
	return &organizationv1.ClusterDetails{
		Id:                c.ID.String(),
		Name:              c.Name,
		Region:            c.Region,
		KubernetesVersion: c.KubernetesVersion,
		Status:            FromClusterStatus(c.Status),
		CreatedAt: &organizationv1.Timestamp{
			Value: c.Created.Time.Format(time.RFC3339),
		},
		ResourceUsage: nil, // Stub
		NodePools:     nil, // Stub
		Members:       nil, // Stub
		Projects:      nil, // Stub
	}
}

func FromClusterStatus(status string) organizationv1.ClusterStatus {
	switch status {
	case "provisioning":
		return organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING
	case "starting":
		return organizationv1.ClusterStatus_CLUSTER_STATUS_STARTING
	case "running":
		return organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING
	case "upgrading":
		return organizationv1.ClusterStatus_CLUSTER_STATUS_UPGRADING
	case "error":
		return organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR
	case "stopping":
		return organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPING
	case "stopped":
		return organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPED
	default:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_UNSPECIFIED
	}
}

func FromNodePools(nodePools []db.TenantNodePool) []*organizationv1.NodePool {
	result := make([]*organizationv1.NodePool, 0, len(nodePools))
	for _, np := range nodePools {
		result = append(result, FromNodePool(np))
	}
	return result
}

func FromNodePool(np db.TenantNodePool) *organizationv1.NodePool {
	return &organizationv1.NodePool{
		Id:           np.ID.String(),
		Name:         np.Name,
		MachineType:  np.MachineType,
		CurrentNodes: 0, // Stub: would come from actual cluster state
		MinNodes:     np.AutoscaleMin,
		MaxNodes:     np.AutoscaleMax,
		Status:       organizationv1.NodePoolStatus_NODE_POOL_STATUS_UNSPECIFIED, // Stub
		Version:      "",                                                         // Stub: would come from actual cluster state
	}
}
