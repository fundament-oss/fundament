package organization

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/clusterstatus"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// clusterStatusFromDB derives the cluster's status the way every service
// does (clusterstatus.FromDB) and maps it onto the console's API.
func clusterStatusFromDB(deleted pgtype.Timestamptz, shootStatus pgtype.Text, shootMessage pgtype.Text, shootHealth pgtype.Text, updating bool, outboxStatus pgtype.Text, reachedGardener bool) organizationv1.ClusterStatus {
	status := clusterstatus.FromDB(clusterstatus.Input{
		Deleted:         deleted,
		ShootStatus:     shootStatus,
		ShootMessage:    shootMessage,
		ShootHealth:     shootHealth,
		Updating:        updating,
		OutboxStatus:    outboxStatus,
		ReachedGardener: reachedGardener,
	})
	switch status {
	case clusterstatus.Unspecified:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_UNSPECIFIED
	case clusterstatus.Provisioning:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING
	case clusterstatus.Starting:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_STARTING
	case clusterstatus.Running:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING
	case clusterstatus.Upgrading:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_UPGRADING
	case clusterstatus.Unhealthy:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_UNHEALTHY
	case clusterstatus.Error:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR
	case clusterstatus.Deleting:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING
	case clusterstatus.Stopped:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPED
	case clusterstatus.Lost:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_LOST
	default:
		panic(fmt.Sprintf("unhandled cluster status %d", status))
	}
}

// syncError is a sync row's status_info when it is an error. A pending row's
// status_info is the precondition it waits on (waiting, not failing); only a
// retrying or failed row carries an error.
func syncError(status, info pgtype.Text) (string, bool) {
	if !info.Valid || (status.String != "retrying" && status.String != "failed") {
		return "", false
	}
	return info.String, true
}

// syncStateFromRow builds the sync state from the cluster's own latest sync
// row. A node pool's sync is the pool's own (nodePoolFromRow).
func syncStateFromRow(
	outboxStatus pgtype.Text,
	outboxRetries int32,
	outboxError pgtype.Text,
	shootStatus pgtype.Text,
	shootStatusMessage pgtype.Text,
	shootStatusUpdated pgtype.Timestamptz,
	shootUpdating bool,
) *organizationv1.SyncState {
	state := organizationv1.SyncState_builder{
		OutboxRetries: outboxRetries,
		ShootUpdating: shootUpdating,
	}.Build()

	if outboxStatus.Valid {
		state.SetOutboxStatus(outboxStatus.String)
	}
	message, isError := syncError(outboxStatus, outboxError)
	if isError {
		state.SetOutboxError(message)
	}
	// A pending row's status_info is not an error but why it waits (a
	// precondition it is parked or deferred on).
	if outboxStatus.Valid && outboxStatus.String == "pending" && outboxError.Valid && outboxError.String != "" {
		state.SetOutboxMessage(outboxError.String)
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
