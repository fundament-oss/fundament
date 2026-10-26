package organization

import (
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/dbconst"
	commongardener "github.com/fundament-oss/fundament/common/gardener"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// clusterStatusFromDB derives cluster status from the deleted flag, Gardener's
// shoot status and the cluster's own sync. A cluster whose sync failed for good
// is in error when Gardener does not have its Shoot: it never reached Gardener
// (no status yet, no sync ever completed) or its Shoot disappeared before a
// status check saw it (pending). Before the first status is recorded a cluster
// that did reach Gardener is still provisioning, whatever later sync failed. A
// sync that fails for a cluster that already runs leaves the status alone; the
// failure shows in the sync state. A cluster whose Shoot disappeared after a
// status check saw it is lost, until a change creates the Shoot again.
// A freshly created cluster whose health checks have not passed yet is
// starting, not unhealthy — the worker marks that window with the
// awaiting-health message. After that, a ready cluster whose health
// conditions fail is unhealthy; that wins over updating, because a stuck
// update is a failure, not progress. A ready cluster that an update fundament
// pushed is rolling out on is upgrading; it stays usable meanwhile.
func clusterStatusFromDB(deleted pgtype.Timestamptz, shootStatus pgtype.Text, shootMessage pgtype.Text, shootHealth pgtype.Text, updating bool, outboxStatus pgtype.Text, reachedGardener bool) organizationv1.ClusterStatus {
	if deleted.Valid {
		return organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING
	}
	syncFailed := outboxStatus.Valid && outboxStatus.String == "failed"
	if !shootStatus.Valid {
		if syncFailed && !reachedGardener {
			return organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR
		}
		return organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING
	}
	if shootStatus.String == "pending" {
		if syncFailed {
			return organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR
		}
		return organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING
	}
	switch shootStatus.String {
	case "progressing":
		return organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING
	case "ready":
		if dbconst.ClusterShootHealth(shootHealth.String) == dbconst.ClusterShootHealth_Unhealthy {
			if shootMessage.String == commongardener.MsgShootAwaitingHealth {
				return organizationv1.ClusterStatus_CLUSTER_STATUS_STARTING
			}
			return organizationv1.ClusterStatus_CLUSTER_STATUS_UNHEALTHY
		}
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
	case "lost":
		return organizationv1.ClusterStatus_CLUSTER_STATUS_LOST
	default:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_UNSPECIFIED
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
