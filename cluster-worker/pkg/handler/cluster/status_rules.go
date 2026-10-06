package cluster

import (
	"fmt"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/common/dbconst"
)

// storedShootState is what the database holds for a cluster before a poll.
type storedShootState struct {
	Status  gardener.ShootStatusType   // empty when never checked
	Message string                     // empty when never checked
	Health  dbconst.ClusterShootHealth // empty when unknown
	// Updating is set while an update fundament pushed rolls out on a ready
	// cluster (the sync handler sets it).
	Updating bool
}

// statusEvent is one activity-log entry a poll records.
type statusEvent struct {
	Type    dbconst.ClusterEventEventType
	Message string
}

// shootStatusUpdate is what a poll writes back for one cluster.
type shootStatusUpdate struct {
	Status  gardener.ShootStatusType
	Message string
	Health  dbconst.ClusterShootHealth // empty writes NULL
	// Updating keeps a ready cluster marked as updating.
	Updating bool
	Events   []statusEvent
	// InsertReady is set on a transition into ready: the ready outbox row fans
	// out to the handlers that provision a freshly ready cluster.
	InsertReady bool
}

// nextShootStatus decides what a status poll writes, given the stored row and
// what Gardener reports. It is pure so every rule can be tested without a
// database.
func nextShootStatus(stored storedShootState, observed *gardener.ShootStatus) shootStatusUpdate {
	// Gardener reconciles running shoots periodically (gardenlet SyncPeriod, 1h
	// by default). That is not a lifecycle change for a ready cluster, so it
	// stays ready, but its health follows the conditions: a reconcile that
	// breaks the cluster, or never finishes, must not look healthy. Updates
	// fundament pushes are marked updating by the sync handler instead.
	if stored.Status == gardener.StatusReady && stored.Updating {
		if update, ok := readyDuringUpdate(observed); ok {
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
	if observed.Status == gardener.StatusError && observed.Retrying {
		warning := shootStatusUpdate{Status: gardener.StatusProgressing, Message: observed.Message}
		if stored.Status == gardener.StatusReady {
			warning.Status = gardener.StatusReady
			warning.Health = stored.Health
			warning.Updating = stored.Updating
		}
		if observed.Message != stored.Message {
			warning.Events = []statusEvent{{Type: dbconst.ClusterEventEventType_StatusWarning, Message: observed.Message}}
		}
		return warning
	}

	update := shootStatusUpdate{Status: observed.Status, Message: observed.Message}
	if observed.Status == gardener.StatusReady {
		update.Health = shootHealth(observed.Healthy)
	}

	if observed.Status != stored.Status {
		if eventType := statusTransitionEvent(observed.Status); eventType != "" {
			update.Events = append(update.Events, statusEvent{Type: eventType, Message: observed.Message})
		}
		update.InsertReady = observed.Status == gardener.StatusReady
		return update
	}

	// Still ready: record health changes. The first recorded health (stored NULL)
	// has nothing to compare to; the status_ready event already carried it.
	if observed.Status == gardener.StatusReady && stored.Health != "" && stored.Health != update.Health {
		update.Events = append(update.Events, healthEvent(update.Health, observed.Message))
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
func readyDuringUpdate(observed *gardener.ShootStatus) (shootStatusUpdate, bool) {
	switch observed.Status {
	case gardener.StatusProgressing:
		return shootStatusUpdate{
			Status:   gardener.StatusReady,
			Message:  observed.Message,
			Health:   shootHealth(observed.Healthy),
			Updating: true,
		}, true
	case gardener.StatusReady:
		return shootStatusUpdate{
			Status:  gardener.StatusReady,
			Message: observed.Message,
			Health:  shootHealth(observed.Healthy),
			Events:  []statusEvent{{Type: dbconst.ClusterEventEventType_StatusReady, Message: observed.Message}},
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
	update := shootStatusUpdate{Status: gardener.StatusReady, Message: stored.Message, Health: shootHealth(observed.Healthy)}
	switch {
	case !observed.Healthy:
		update.Message = observed.Message
	case stored.Health != dbconst.ClusterShootHealth_Healthy:
		update.Message = gardener.MsgShootReady
	}
	if stored.Health != "" && stored.Health != update.Health {
		update.Events = []statusEvent{healthEvent(update.Health, update.Message)}
	}
	return update
}

// healthEvent is the event recorded when a ready cluster's health changes.
func healthEvent(health dbconst.ClusterShootHealth, message string) statusEvent {
	if health == dbconst.ClusterShootHealth_Unhealthy {
		return statusEvent{Type: dbconst.ClusterEventEventType_StatusUnhealthy, Message: message}
	}
	return statusEvent{Type: dbconst.ClusterEventEventType_StatusHealthy, Message: message}
}

// shootWasSeen reports whether a stored status can only come from a poll that
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
		// Handled in pollDeletedClusters
		return ""
	default:
		panic(fmt.Sprintf("unhandled shoot status: %s", status))
	}
}
