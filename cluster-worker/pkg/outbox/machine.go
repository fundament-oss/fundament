package outbox

import (
	"github.com/fundament-oss/fundament/cluster-worker/pkg/machine"
	"github.com/fundament-oss/fundament/common/dbconst"
)

// Event is the outcome of one attempt at an outbox row.
type Event string

const (
	// EventCompleted is a successful sync: the row is done.
	EventCompleted Event = "completed"
	// EventRetried is a failed attempt that will be tried again.
	EventRetried Event = "retried"
	// EventFailed is a failure a retry cannot fix, or the last retry.
	EventFailed Event = "failed"
)

// Machine is the outbox row lifecycle. Its edges are enforced twice: here, by
// validation at start-up and by the tests below, and in pkg/db/outbox.sql,
// where every status write carries the status it expects. A worker that lost a
// row to another worker leaves it alone instead of overwriting the outcome.
var Machine = machine.Must(machine.New(
	"cluster-outbox",
	dbconst.ClusterOutboxStatus_Pending,
	machine.Transition[dbconst.ClusterOutboxStatus, Event]{
		From: dbconst.ClusterOutboxStatus_Pending, Event: EventCompleted, To: dbconst.ClusterOutboxStatus_Completed,
	},
	machine.Transition[dbconst.ClusterOutboxStatus, Event]{
		From: dbconst.ClusterOutboxStatus_Pending, Event: EventRetried, To: dbconst.ClusterOutboxStatus_Retrying,
	},
	machine.Transition[dbconst.ClusterOutboxStatus, Event]{
		From: dbconst.ClusterOutboxStatus_Pending, Event: EventFailed, To: dbconst.ClusterOutboxStatus_Failed,
	},
	machine.Transition[dbconst.ClusterOutboxStatus, Event]{
		From: dbconst.ClusterOutboxStatus_Retrying, Event: EventCompleted, To: dbconst.ClusterOutboxStatus_Completed,
	},
	machine.Transition[dbconst.ClusterOutboxStatus, Event]{
		From: dbconst.ClusterOutboxStatus_Retrying, Event: EventRetried, To: dbconst.ClusterOutboxStatus_Retrying,
	},
	machine.Transition[dbconst.ClusterOutboxStatus, Event]{
		From: dbconst.ClusterOutboxStatus_Retrying, Event: EventFailed, To: dbconst.ClusterOutboxStatus_Failed,
	},
).Terminal(
	dbconst.ClusterOutboxStatus_Completed,
	dbconst.ClusterOutboxStatus_Failed,
))
