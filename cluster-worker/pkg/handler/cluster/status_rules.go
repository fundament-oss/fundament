package cluster

import (
	"fmt"
	"time"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/common/dbconst"
)

// storedShootState is what the database holds for a cluster before a status check.
type storedShootState struct {
	Status  gardener.ShootStatusType   // empty when never checked
	Message string                     // empty when never checked
	Health  dbconst.ClusterShootHealth // empty when unknown
	// Updating is set while an update fundament pushed rolls out on a ready
	// cluster (the sync handler sets it).
	Updating bool
	// LastWarning is the message of the latest status_warning event. It is
	// only loaded when Gardener reports an error it retries.
	LastWarning string
}

// progressRecord is the cluster's latest status_progressing event: when, and
// which message. Zero when there is none.
type progressRecord struct {
	At      time.Time
	Message string
}

// statusEvent is one activity-log entry a status check records.
type statusEvent struct {
	Type    dbconst.ClusterEventEventType
	Message string
}

// shootStatusUpdate is what a status check writes back for one cluster.
type shootStatusUpdate struct {
	Status  gardener.ShootStatusType
	Message string
	Health  dbconst.ClusterShootHealth // empty writes NULL
	// Updating keeps a ready cluster marked as updating.
	Updating bool
	// Warning is set when the message is an error Gardener retries; it is
	// recorded as a warning, never as progress.
	Warning bool
	Events  []statusEvent
	// InsertReady is set on a transition into ready: the ready outbox row fans
	// out to the handlers that provision a freshly ready cluster.
	InsertReady bool
}

// addProgressEvent records Gardener's progress message while an operation
// runs: while the cluster is progressing, or ready and updating. The
// description names the running tasks and changes after every task, about a
// hundred times per operation, so a new message is recorded at most once per
// interval. A change inside the window is not dropped: the
// result says how long to wait, and the check then records the message that
// is current if it still differs, so a reconcile stuck on one task records
// that task. A transition into progressing already records its message.
func addProgressEvent(update *shootStatusUpdate, last progressRecord, now time.Time, interval time.Duration) (retryAfter time.Duration) {
	// A warning's message is Gardener's error, recorded as the warning, also
	// when the same error repeats and records no second one.
	if (update.Status != gardener.StatusProgressing && !update.Updating) || update.Warning || update.Message == last.Message {
		return 0
	}
	for _, event := range update.Events {
		// A transition into progressing already records its message.
		if event.Type == dbconst.ClusterEventEventType_StatusProgressing {
			return 0
		}
	}
	if wait := interval - now.Sub(last.At); wait > 0 {
		return wait
	}
	update.Events = append(update.Events, statusEvent{Type: dbconst.ClusterEventEventType_StatusProgressing, Message: update.Message})
	return 0
}

// unchanged reports whether writing the update would change nothing: same
// status, message and health as stored, and nothing to record.
func (u *shootStatusUpdate) unchanged(stored storedShootState) bool {
	return u.Status == stored.Status && u.Message == stored.Message && u.Health == stored.Health &&
		len(u.Events) == 0 && !u.InsertReady
}

// nextShootStatus decides what a status check writes, given the stored row and
// what Gardener reports. It is pure so every rule can be tested without a
// database.
func nextShootStatus(stored storedShootState, observed *gardener.ShootStatus) shootStatusUpdate {
	// Gardener reconciles running shoots periodically (gardenlet SyncPeriod, 1h
	// by default). That is not a lifecycle change for a ready cluster, so it
	// stays ready, but its health follows Gardener's verdict: a reconcile that
	// breaks the cluster, or never finishes, must not look healthy. Updates
	// fundament pushes are marked updating by the sync handler instead.
	if stored.Status == gardener.StatusReady && stored.Updating {
		if update, ok := readyDuringUpdate(stored.Health, observed); ok {
			return update
		}
	}
	if stored.Status == gardener.StatusReady &&
		observed.Status == gardener.StatusProgressing &&
		observed.Operation == gardener.OperationReconcile {
		return readyDuringReconcile(stored, observed)
	}

	// A Shoot that disappears after fundament has seen it was lost, not "not
	// created yet": record that, since drift reconciliation will recreate it
	// and the history would otherwise show a fresh create with no reason.
	if observed.Status == gardener.StatusPending && observed.Message == gardener.MsgShootNotFound && shootWasSeen(stored.Status) {
		return shootStatusUpdate{
			Status:  gardener.StatusPending,
			Message: observed.Message,
			Events:  []statusEvent{{Type: dbconst.ClusterEventEventType_StatusLost, Message: observed.Message}},
		}
	}

	// Errors Gardener will retry by itself are a warning, not a failure: keep the
	// cluster where it was (ready, or progressing while it is being created),
	// show Gardener's message and record one status_warning per distinct message.
	// Gardener retries such an error again and again, with progress in between
	// that the watch now sees, so the comparison is with the last recorded
	// warning, not with the stored message.
	if observed.Status == gardener.StatusError && observed.Retrying {
		warning := shootStatusUpdate{Status: gardener.StatusProgressing, Message: observed.Message, Warning: true}
		if stored.Status == gardener.StatusReady {
			warning.Status = gardener.StatusReady
			warning.Health = stored.Health
			warning.Updating = stored.Updating
		}
		if observed.Message != stored.LastWarning {
			warning.Events = []statusEvent{{Type: dbconst.ClusterEventEventType_StatusWarning, Message: observed.Message}}
		}
		return warning
	}

	update := shootStatusUpdate{Status: observed.Status, Message: observed.Message}
	if observed.Status == gardener.StatusReady {
		update.Health = healthOf(stored.Health, observed)
		update.Message = readyMessage(update.Health)
		if settlingAfterCreate(stored, observed, update.Health) {
			update.Message = gardener.MsgShootAwaitingHealth
		}
	}

	if observed.Status != stored.Status {
		if eventType := statusTransitionEvent(observed.Status); eventType != "" {
			update.Events = append(update.Events, statusEvent{Type: eventType, Message: update.Message})
		}
		update.InsertReady = observed.Status == gardener.StatusReady
		return update
	}

	// Still ready: record health changes. The first recorded health (stored NULL)
	// has nothing to compare to; the status_ready event already carried it.
	if observed.Status == gardener.StatusReady && stored.Health != "" && stored.Health != update.Health {
		update.Events = append(update.Events, healthEvent(update.Health, update.Message))
	}

	return update
}

// readyDuringUpdate follows an update fundament pushed to a ready cluster.
// The cluster stays ready, so it stays usable and the ready fan-out does not
// run again, and stays marked as updating, with Gardener's progress as its
// message, until the reconcile finishes; that records status_ready. Health is
// tracked, but a rolling update takes conditions down on purpose, so its
// changes record no events. Anything else (a failure, a lost or deleted
// Shoot) is not handled here and ends the update.
func readyDuringUpdate(storedHealth dbconst.ClusterShootHealth, observed *gardener.ShootStatus) (shootStatusUpdate, bool) {
	switch observed.Status {
	case gardener.StatusProgressing:
		return shootStatusUpdate{
			Status:   gardener.StatusReady,
			Message:  observed.Message,
			Health:   healthOf(storedHealth, observed),
			Updating: true,
		}, true
	case gardener.StatusReady:
		health := healthOf(storedHealth, observed)
		return shootStatusUpdate{
			Status:  gardener.StatusReady,
			Message: readyMessage(health),
			Health:  health,
			Events:  []statusEvent{{Type: dbconst.ClusterEventEventType_StatusReady, Message: readyMessage(health)}},
		}, true
	case gardener.StatusPending, gardener.StatusError, gardener.StatusDeleting, gardener.StatusDeleted:
		return shootStatusUpdate{}, false
	default:
		panic(fmt.Sprintf("unhandled shoot status: %s", observed.Status))
	}
}

// readyDuringReconcile keeps a ready cluster ready while Gardener reconciles
// it. While it is healthy the stored message stays; while it is not, Gardener's
// progress message says what the reconcile is waiting for.
func readyDuringReconcile(stored storedShootState, observed *gardener.ShootStatus) shootStatusUpdate {
	update := shootStatusUpdate{Status: gardener.StatusReady, Message: stored.Message, Health: healthOf(stored.Health, observed)}
	switch {
	case update.Health != dbconst.ClusterShootHealth_Healthy:
		update.Message = observed.Message
	case stored.Health != dbconst.ClusterShootHealth_Healthy:
		update.Message = gardener.MsgShootReady
	}
	if stored.Health != "" && stored.Health != update.Health {
		update.Events = []statusEvent{healthEvent(update.Health, update.Message)}
	}
	return update
}

// healthOf maps Gardener's health verdict to shoot_health. Within Gardener's
// grace period (status label "progressing") the stored health stays, so a
// condition that turns bad briefly does not flip it; a cluster that was never
// healthy is not called healthy before Gardener does.
func healthOf(stored dbconst.ClusterShootHealth, observed *gardener.ShootStatus) dbconst.ClusterShootHealth {
	if observed.HealthGrace {
		if stored != "" {
			return stored
		}
		return dbconst.ClusterShootHealth_Unhealthy
	}
	return shootHealth(observed.Healthy)
}

// settlingAfterCreate reports whether a ready, not yet healthy cluster is still
// starting up: Gardener's last operation is the create, and the cluster has not
// been healthy since. A new cluster's conditions take a few minutes to pass,
// and Gardener labels it unhealthy meanwhile; that is not a fault yet. The
// stored message tells whether it has been healthy: once healthy it reads
// MsgShootReady, so a later failure is reported as unhealthy.
func settlingAfterCreate(stored storedShootState, observed *gardener.ShootStatus, health dbconst.ClusterShootHealth) bool {
	if observed.Operation != gardener.OperationCreate || health == dbconst.ClusterShootHealth_Healthy {
		return false
	}
	return stored.Status != gardener.StatusReady || stored.Message == gardener.MsgShootAwaitingHealth
}

// readyMessage is the message of a ready cluster with the given health.
func readyMessage(health dbconst.ClusterShootHealth) string {
	if health == dbconst.ClusterShootHealth_Healthy {
		return gardener.MsgShootReady
	}
	return gardener.MsgShootUnhealthy
}

// healthEvent is the event recorded when a ready cluster's health changes.
func healthEvent(health dbconst.ClusterShootHealth, message string) statusEvent {
	if health == dbconst.ClusterShootHealth_Unhealthy {
		return statusEvent{Type: dbconst.ClusterEventEventType_StatusUnhealthy, Message: message}
	}
	return statusEvent{Type: dbconst.ClusterEventEventType_StatusHealthy, Message: message}
}

// shootWasSeen reports whether a stored status can only come from a check that
// found the Shoot in Gardener.
func shootWasSeen(status gardener.ShootStatusType) bool {
	switch status {
	case gardener.StatusProgressing, gardener.StatusReady, gardener.StatusError, gardener.StatusDeleting:
		return true
	case "", gardener.StatusPending, gardener.StatusDeleted:
		return false
	default:
		panic(fmt.Sprintf("unhandled shoot status: %s", status))
	}
}

func shootHealth(healthy bool) dbconst.ClusterShootHealth {
	if healthy {
		return dbconst.ClusterShootHealth_Healthy
	}
	return dbconst.ClusterShootHealth_Unhealthy
}

// statusTransitionEvent returns the event recorded when a cluster enters the
// given status, or empty for statuses that record none.
func statusTransitionEvent(status gardener.ShootStatusType) dbconst.ClusterEventEventType {
	switch status {
	case gardener.StatusProgressing:
		return dbconst.ClusterEventEventType_StatusProgressing
	case gardener.StatusReady:
		return dbconst.ClusterEventEventType_StatusReady
	case gardener.StatusError:
		return dbconst.ClusterEventEventType_StatusError
	case gardener.StatusPending, gardener.StatusDeleting:
		// No event for these transient states
		return ""
	case gardener.StatusDeleted:
		// Handled in checkDeletedCluster
		return ""
	default:
		panic(fmt.Sprintf("unhandled shoot status: %s", status))
	}
}
