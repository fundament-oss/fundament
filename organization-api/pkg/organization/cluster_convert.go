package organization

import (
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// clusterStatusFromDB derives cluster status from deleted flag + Gardener shoot status.
func clusterStatusFromDB(deleted pgtype.Timestamptz, shootStatus pgtype.Text) organizationv1.ClusterStatus {
	if deleted.Valid {
		return organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING
	}
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

func syncStateFromRow(
	synced pgtype.Timestamptz,
	syncError pgtype.Text,
	syncAttempts int32,
	shootStatus pgtype.Text,
	shootStatusMessage pgtype.Text,
	shootStatusUpdated pgtype.Timestamptz,
) *organizationv1.SyncState {
	state := organizationv1.SyncState_builder{
		SyncAttempts: syncAttempts,
	}.Build()

	if synced.Valid {
		state.SetSyncedAt(timestamppb.New(synced.Time))
	}
	if syncError.Valid {
		state.SetSyncError(syncError.String)
	}
	if shootStatus.Valid {
		state.SetShootStatus(shootStatus.String)
	}
	if shootStatusMessage.Valid {
		state.SetShootMessage(shootStatusMessage.String)
	}
	if shootStatusUpdated.Valid {
		state.SetStatusUpdatedAt(timestamppb.New(shootStatusUpdated.Time))
	}

	return state
}

func clusterEventsFromRows(events []db.TenantClusterEvent) []*organizationv1.ClusterEvent {
	result := make([]*organizationv1.ClusterEvent, 0, len(events))
	for i := range events {
		result = append(result, clusterEventFromRow(&events[i]))
	}
	return result
}

func clusterEventFromRow(e *db.TenantClusterEvent) *organizationv1.ClusterEvent {
	event := organizationv1.ClusterEvent_builder{
		Id:        e.ID.String(),
		EventType: string(e.EventType),
		CreatedAt: timestamppb.New(e.Created.Time),
	}.Build()

	if e.SyncAction.Valid {
		event.SetSyncAction(e.SyncAction.String)
	}
	if e.Message.Valid {
		event.SetMessage(e.Message.String)
	}
	if e.Attempt.Valid {
		event.SetAttempt(e.Attempt.Int32)
	}

	return event
}
