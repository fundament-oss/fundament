package cluster

import (
	"time"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/common/dbconst"
)

// The messages a cluster stuck past a timeout records. They are constants so
// that the same warning is recorded once, not on every check.
const (
	// msgShootNeverAppeared is also an identity: nextLifecycleStatus matches a
	// stored error against it to tell a cluster that never had a Shoot from one
	// whose Shoot disappeared. Rewording it strands clusters already in this
	// state, whose next not-found would read as lost.
	msgShootNeverAppeared = "The Shoot did not appear in Gardener; the cluster's next change tries again."
	msgUpdateStalled      = "The update has not finished long after the change reached Gardener."
	msgDeleteStuck        = "The Shoot has been deleting for a long time."
)

// statusTimeouts are the limits after which a cluster that stopped moving is
// reported. Waiting does not change any of these conditions, so each ends in a
// visible state rather than staying quiet: a pending Shoot becomes error, a
// stuck update or deletion records a warning, and a stuck deletion also alerts.
type statusTimeouts struct {
	pending  time.Duration
	updating time.Duration
	deleting time.Duration
}

// timedOutPending replaces the check's decision for a Shoot Gardener never
// created: after the limit the cluster reads error, so the console stops
// showing a create that nothing is running. A cluster whose Shoot once existed
// is not this case: a pending row that carries Gardener's own report says the
// Shoot was there, and its not-found is lost, handled by the lifecycle rules.
func timedOutPending(stored *storedShootState, observed *gardener.ShootStatus, now time.Time, limit time.Duration) (shootStatusUpdate, bool) {
	if observed.Status != gardener.StatusPending || observed.Message != gardener.MsgShootNotFound {
		return shootStatusUpdate{}, false
	}
	if stored.Status != gardener.StatusPending || stored.Message != gardener.MsgShootNotFound ||
		stored.Since.IsZero() || now.Sub(stored.Since) <= limit {
		return shootStatusUpdate{}, false
	}
	return shootStatusUpdate{
		Status:  gardener.StatusError,
		Message: msgShootNeverAppeared,
		Events:  []statusEvent{{Type: dbconst.ClusterEventEventType_StatusError, Message: msgShootNeverAppeared}},
	}, true
}

// addUpdateWarning records a warning when an update fundament pushed settled
// in the outbox but Gardener has still not finished it. The updating flag is
// left alone: clearing it would hide a genuinely stuck update.
func addUpdateWarning(update *shootStatusUpdate, stored *storedShootState, now time.Time, limit time.Duration) bool {
	if !update.Updating || stored.UpdateSettled.IsZero() || now.Sub(stored.UpdateSettled) <= limit {
		return false
	}
	return addWarningOnce(update, stored, msgUpdateStalled)
}

// addDeletingWarning records a warning when a Shoot has been deleting past its
// limit. It reports whether the warning is new, which is when the caller
// alerts; the check runs again and again while the Shoot stays.
func addDeletingWarning(update *shootStatusUpdate, stored *storedShootState, now time.Time, limit time.Duration) bool {
	if update.Status != gardener.StatusDeleting || !deletingTooLong(stored, now, limit) {
		return false
	}
	return addWarningOnce(update, stored, msgDeleteStuck)
}

// deletingTooLong reports whether the stored deleting status has lasted past
// its limit.
func deletingTooLong(stored *storedShootState, now time.Time, limit time.Duration) bool {
	return stored.Status == gardener.StatusDeleting && !stored.Since.IsZero() && now.Sub(stored.Since) > limit
}

// addWarningOnce appends a status_warning event unless the cluster already
// recorded that message since its last milestone, so a stuck state is recorded
// once per incident rather than on every check.
func addWarningOnce(update *shootStatusUpdate, stored *storedShootState, message string) bool {
	if stored.Warnings[message] {
		return false
	}
	update.Events = append(update.Events, statusEvent{Type: dbconst.ClusterEventEventType_StatusWarning, Message: message})
	return true
}
