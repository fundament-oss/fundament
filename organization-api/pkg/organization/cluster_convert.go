package organization

import (
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func clusterStatusFromDB(status string) organizationv1.ClusterStatus {
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

func syncStateFromRow(
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
		state.SyncedAt = timestamppb.New(synced.Time)
	}
	if syncError.Valid {
		state.SyncError = &syncError.String
	}
	if syncAttempts.Valid {
		state.SyncAttempts = syncAttempts.Int32
	}
	if syncLastAttempt.Valid {
		state.LastAttemptAt = timestamppb.New(syncLastAttempt.Time)
	}
	if shootStatus.Valid {
		state.ShootStatus = &shootStatus.String
	}
	if shootStatusMessage.Valid {
		state.ShootMessage = &shootStatusMessage.String
	}
	if shootStatusUpdated.Valid {
		state.StatusUpdatedAt = timestamppb.New(shootStatusUpdated.Time)
	}

	return state
}
