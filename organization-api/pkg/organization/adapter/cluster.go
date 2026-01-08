package adapter

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/models"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func ToClusterCreate(req *organizationv1.CreateClusterRequest) models.ClusterCreate {
	return models.ClusterCreate{
		Name:              req.Name,
		Region:            req.Region,
		KubernetesVersion: req.KubernetesVersion,
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

func FromClustersSummary(clusters []db.ClusterListByOrganizationIDRow) []*organizationv1.ClusterSummary {
	summaries := make([]*organizationv1.ClusterSummary, 0, len(clusters))
	for _, c := range clusters {
		summaries = append(summaries, FromClusterSummary(c))
	}
	return summaries
}

func FromClusterSummary(c db.ClusterListByOrganizationIDRow) *organizationv1.ClusterSummary {
	return &organizationv1.ClusterSummary{
		Id:            c.ID.String(),
		Name:          c.Name,
		Status:        FromClusterStatus(c.Status),
		Region:        c.Region,
		ProjectCount:  0, // Stub
		NodePoolCount: 0, // Stub
		SyncState:     FromSyncState(c.Synced, c.SyncError, c.SyncAttempts, c.SyncLastAttempt, c.ShootStatus, c.ShootStatusMessage, c.ShootStatusUpdated),
	}
}

func FromClusterDetail(c db.ClusterGetByIDRow) *organizationv1.ClusterDetails {
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
		SyncState:     FromSyncState(c.Synced, c.SyncError, c.SyncAttempts, c.SyncLastAttempt, c.ShootStatus, c.ShootStatusMessage, c.ShootStatusUpdated),
	}
}

// FromClusterDetailBasic converts a TenantCluster (without sync state) to ClusterDetails.
// Used for update responses where we don't have sync state.
func FromClusterDetailBasic(c db.TenantCluster) *organizationv1.ClusterDetails {
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
		SyncState:     nil, // Not available for basic cluster data
	}
}

func FromSyncState(
	synced pgtype.Timestamptz,
	syncError pgtype.Text,
	syncAttempts pgtype.Int4,
	syncLastAttempt pgtype.Timestamptz,
	shootStatus pgtype.Text,
	shootStatusMessage pgtype.Text,
	shootStatusUpdated pgtype.Timestamptz,
) *organizationv1.SyncState {
	state := &organizationv1.SyncState{}

	if synced.Valid {
		state.SyncedAt = &organizationv1.Timestamp{Value: synced.Time.Format(time.RFC3339)}
	}
	if syncError.Valid {
		state.SyncError = &syncError.String
	}
	if syncAttempts.Valid {
		state.SyncAttempts = syncAttempts.Int32
	}
	if syncLastAttempt.Valid {
		state.LastAttemptAt = &organizationv1.Timestamp{Value: syncLastAttempt.Time.Format(time.RFC3339)}
	}
	if shootStatus.Valid {
		state.ShootStatus = &shootStatus.String
	}
	if shootStatusMessage.Valid {
		state.ShootMessage = &shootStatusMessage.String
	}
	if shootStatusUpdated.Valid {
		state.StatusUpdatedAt = &organizationv1.Timestamp{Value: shootStatusUpdated.Time.Format(time.RFC3339)}
	}

	return state
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

func FromClusterNamespaces(namespaces []db.TenantNamespace) []*organizationv1.ClusterNamespace {
	result := make([]*organizationv1.ClusterNamespace, 0, len(namespaces))
	for i := range namespaces {
		result = append(result, FromClusterNamespace(&namespaces[i]))
	}
	return result
}

func FromClusterNamespace(ns *db.TenantNamespace) *organizationv1.ClusterNamespace {
	return &organizationv1.ClusterNamespace{
		Id:        ns.ID.String(),
		Name:      ns.Name,
		ProjectId: ns.ProjectID.String(),
		CreatedAt: &organizationv1.Timestamp{
			Value: ns.Created.Time.Format(time.RFC3339),
		},
	}
}
