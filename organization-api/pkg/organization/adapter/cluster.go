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
	for i := range clusters {
		summaries = append(summaries, FromClusterSummary(&clusters[i]))
	}
	return summaries
}

func FromClusterSummary(c *db.ClusterListByOrganizationIDRow) *organizationv1.ClusterSummary {
	return &organizationv1.ClusterSummary{
		Id:            c.ID.String(),
		Name:          c.Name,
		Status:        StatusFromCluster(c.Deleted, c.ShootStatus),
		Region:        c.Region,
		ProjectCount:  0, // Stub
		NodePoolCount: 0, // Stub
		SyncState:     FromSyncState(c.Synced, c.SyncError, c.SyncAttempts, c.ShootStatus, c.ShootStatusMessage, c.ShootStatusUpdated),
	}
}

func FromClusterDetail(c *db.ClusterGetByIDRow) *organizationv1.ClusterDetails {
	return &organizationv1.ClusterDetails{
		Id:                c.ID.String(),
		Name:              c.Name,
		Region:            c.Region,
		KubernetesVersion: c.KubernetesVersion,
		Status:            StatusFromCluster(c.Deleted, c.ShootStatus),
		CreatedAt: &organizationv1.Timestamp{
			Value: c.Created.Time.Format(time.RFC3339),
		},
		ResourceUsage: nil, // Stub: would come from actual cluster metrics
		SyncState:     FromSyncState(c.Synced, c.SyncError, c.SyncAttempts, c.ShootStatus, c.ShootStatusMessage, c.ShootStatusUpdated),
	}
}

func FromSyncState(
	synced pgtype.Timestamptz,
	syncError pgtype.Text,
	syncAttempts int32,
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
	state.SyncAttempts = syncAttempts
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

// StatusFromCluster derives ClusterStatus from the cluster's deleted flag and Gardener's shoot_status.
// If the cluster is soft-deleted (deleted IS NOT NULL), it's in DELETING state.
// Otherwise, the status is derived from Gardener's shoot_status.
func StatusFromCluster(deleted pgtype.Timestamptz, shootStatus pgtype.Text) organizationv1.ClusterStatus {
	// If cluster is soft-deleted, it's being deleted
	if deleted.Valid {
		return organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING
	}
	return StatusFromShootStatus(shootStatus)
}

// StatusFromShootStatus derives ClusterStatus from Gardener's shoot_status.
// This is the source of truth for cluster state since clusters.status is not updated.
func StatusFromShootStatus(shootStatus pgtype.Text) organizationv1.ClusterStatus {
	if !shootStatus.Valid {
		return organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING
	}
	switch shootStatus.String {
	case "pending", "progressing":
		return organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING
	case "ready":
		return organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING
	case "error":
		return organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR
	case "deleting":
		return organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING
	case "deleted":
		return organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPED
	default:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_UNSPECIFIED
	}
}

func FromNodePools(nodePools []db.TenantNodePool) []*organizationv1.NodePool {
	result := make([]*organizationv1.NodePool, 0, len(nodePools))
	for i := range nodePools {
		result = append(result, FromNodePool(&nodePools[i]))
	}
	return result
}

func FromNodePool(np *db.TenantNodePool) *organizationv1.NodePool {
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

func FromClusterEvents(events []db.TenantClusterEvent) []*organizationv1.ClusterEvent {
	result := make([]*organizationv1.ClusterEvent, 0, len(events))
	for i := range events {
		result = append(result, FromClusterEvent(&events[i]))
	}
	return result
}

func FromClusterEvent(e *db.TenantClusterEvent) *organizationv1.ClusterEvent {
	event := &organizationv1.ClusterEvent{
		Id:        e.ID.String(),
		EventType: string(e.EventType),
		CreatedAt: &organizationv1.Timestamp{Value: e.Created.Time.Format(time.RFC3339)},
	}

	if e.SyncAction.Valid {
		s := string(e.SyncAction.TenantClusterSyncAction)
		event.SyncAction = &s
	}
	if e.Message.Valid {
		event.Message = &e.Message.String
	}
	if e.Attempt.Valid {
		event.Attempt = &e.Attempt.Int32
	}

	return event
}
