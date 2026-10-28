// Package clusterstatus derives a cluster's status from what the database
// holds about it. The console (organization-api) and the staff fleet view
// (platform-api) both read it from here, so a tenant and staff never see a
// different status for the same cluster. Each maps Status onto its own API.
package clusterstatus

import (
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/common/dbconst"
	commongardener "github.com/fundament-oss/fundament/common/gardener"
)

// Status is a cluster's status, independent of any API.
type Status int

const (
	Unspecified Status = iota
	Provisioning
	Starting
	Running
	Upgrading
	Unhealthy
	Error
	Deleting
	Stopped
	Lost
)

// Input is what the status is derived from: the cluster row, the cluster's own
// latest sync row (view cluster_sync_state) and whether a sync of it ever
// completed.
type Input struct {
	Deleted      pgtype.Timestamptz
	ShootStatus  pgtype.Text
	ShootMessage pgtype.Text
	ShootHealth  pgtype.Text
	Updating     bool
	OutboxStatus pgtype.Text
	// ReachedGardener is set once a sync of the cluster completed.
	ReachedGardener bool
}

// FromDB derives the status from the deleted flag, Gardener's shoot status
// and the cluster's own sync. A cluster whose sync failed for good is in error
// when Gardener does not have its Shoot: it never reached Gardener (no status
// yet, no sync ever completed) or its Shoot disappeared before a status check
// saw it (pending). Before the first status is recorded a cluster that did
// reach Gardener is still provisioning, whatever later sync failed. A sync
// that fails for a cluster that already runs leaves the status alone; the
// failure shows in the sync state. A cluster whose Shoot disappeared after a
// status check saw it is lost, until a change creates the Shoot again.
// A freshly created cluster whose health checks have not passed yet is
// starting, not unhealthy — the worker marks that window with the
// awaiting-health message. After that, a ready cluster whose health
// conditions fail is unhealthy; that wins over updating, because a stuck
// update is a failure, not progress. A ready cluster that an update fundament
// pushed is rolling out on is upgrading; it stays usable meanwhile.
func FromDB(in Input) Status {
	if in.Deleted.Valid {
		return Deleting
	}
	syncFailed := in.OutboxStatus.Valid && in.OutboxStatus.String == "failed"
	if !in.ShootStatus.Valid {
		if syncFailed && !in.ReachedGardener {
			return Error
		}
		return Provisioning
	}
	if in.ShootStatus.String == "pending" {
		if syncFailed {
			return Error
		}
		return Provisioning
	}
	switch in.ShootStatus.String {
	case "progressing":
		return Provisioning
	case "ready":
		if dbconst.ClusterShootHealth(in.ShootHealth.String) == dbconst.ClusterShootHealth_Unhealthy {
			if in.ShootMessage.String == commongardener.MsgShootAwaitingHealth {
				return Starting
			}
			return Unhealthy
		}
		if in.Updating {
			return Upgrading
		}
		return Running
	case "error":
		return Error
	case "deleting":
		return Deleting
	case "deleted":
		return Stopped
	case "lost":
		return Lost
	default:
		return Unspecified
	}
}

// NeedsAttention reports the statuses staff look at first: unhealthy, in
// error, or lost.
func (s Status) NeedsAttention() bool {
	return s == Unhealthy || s == Error || s == Lost
}
