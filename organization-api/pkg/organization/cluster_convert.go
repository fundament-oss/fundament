package organization

import (
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// clusterStatusFromDB derives cluster status from the deleted flag, Gardener's
// shoot status and the cluster's own sync. A ready cluster that an update
// fundament pushed is rolling out on is upgrading; it stays usable meanwhile.
// A cluster whose Shoot Gardener does not have (never created, or lost) and
// whose sync failed for good is in error: nothing will create it. A sync that
// fails for a cluster that already runs leaves the status alone; the failure
// shows in the sync state.
func clusterStatusFromDB(deleted pgtype.Timestamptz, shootStatus pgtype.Text, updating bool, outboxStatus pgtype.Text) organizationv1.ClusterStatus {
	if deleted.Valid {
		return organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING
	}
	if !shootStatus.Valid || shootStatus.String == "pending" {
		if outboxStatus.Valid && outboxStatus.String == "failed" {
			return organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR
		}
		return organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING
	}
	switch shootStatus.String {
	case "progressing":
		return organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING
	case "ready":
		if updating {
			return organizationv1.ClusterStatus_CLUSTER_STATUS_UPGRADING
		}
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

// syncStateFromRow builds the sync state from the cluster's own latest sync
// row and, when one of its node pools has a sync that failed for good, that
// pool's name and error.
func syncStateFromRow(
	outboxStatus pgtype.Text,
	outboxRetries int32,
	outboxError pgtype.Text,
	failedNodePoolName pgtype.Text,
	failedNodePoolError pgtype.Text,
	shootStatus pgtype.Text,
	shootStatusMessage pgtype.Text,
	shootStatusUpdated pgtype.Timestamptz,
) *organizationv1.SyncState {
	state := organizationv1.SyncState_builder{
		OutboxRetries: outboxRetries,
	}.Build()

	if outboxStatus.Valid {
		state.SetOutboxStatus(outboxStatus.String)
	}
	// A pending row's status_info is the precondition it waits on (waiting,
	// not failing); only a retrying or failed row carries an error.
	if outboxError.Valid && (outboxStatus.String == "retrying" || outboxStatus.String == "failed") {
		state.SetOutboxError(outboxError.String)
	}
	if failedNodePoolName.Valid {
		state.SetFailedNodePoolName(failedNodePoolName.String)
	}
	if failedNodePoolError.Valid {
		state.SetFailedNodePoolError(failedNodePoolError.String)
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
