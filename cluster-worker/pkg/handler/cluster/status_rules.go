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
	Events  []statusEvent
	// InsertReady is set on a transition into ready: the ready outbox row fans
	// out to the handlers that provision a freshly ready cluster.
	InsertReady bool
}

// nextShootStatus decides what a status poll writes, given the stored row and
// what Gardener reports. It is pure so every rule can be tested without a
// database.
func nextShootStatus(stored storedShootState, observed *gardener.ShootStatus) shootStatusUpdate {
	// Gardener reconciles running shoots periodically (gardenlet SyncPeriod, 1h
	// by default), and a node-pool edit is a reconcile too. Neither is a
	// lifecycle change for a ready cluster: keep status, message and health.
	if stored.Status == gardener.StatusReady &&
		observed.Status == gardener.StatusProgressing &&
		observed.Operation == gardener.OperationReconcile {
		return shootStatusUpdate{Status: stored.Status, Message: stored.Message, Health: stored.Health}
	}

	// Errors Gardener will retry by itself are a warning, not a failure: keep the
	// cluster where it was (ready, or progressing while it is being created),
	// show Gardener's message and record one status_warning per distinct message.
	if observed.Status == gardener.StatusError && observed.Retrying {
		warning := shootStatusUpdate{Status: gardener.StatusProgressing, Message: observed.Message}
		if stored.Status == gardener.StatusReady {
			warning.Status = gardener.StatusReady
			warning.Health = stored.Health
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
		eventType := dbconst.ClusterEventEventType_StatusHealthy
		if update.Health == dbconst.ClusterShootHealth_Unhealthy {
			eventType = dbconst.ClusterEventEventType_StatusUnhealthy
		}
		update.Events = append(update.Events, statusEvent{Type: eventType, Message: observed.Message})
	}

	return update
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
