package cluster

import (
	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/machine"
)

// shootUnchecked is the stored shoot_status of a cluster no check has recorded
// yet.
const shootUnchecked gardener.ShootStatusType = ""

// shootLifecycle is the set of shoot_status changes a check may record: From
// is the stored status, the event is the status the check decides to write, and
// To is that same status. A transition table keyed by semantic events would
// have to restate the guards nextLifecycleStatus is made of; keyed by the
// decision, the table says exactly what is allowed and the test can assert
// every emitted update lands on it.
//
// Gardener can report any of its states at any time — an error while
// progressing, a migrate that sends a ready shoot back to progressing, a Shoot
// recreated after a change that starts pending again — and a deleted cluster is
// checked on its own path, which writes deleting or deleted whatever status the
// row holds. So every state leads to every status. The set's value is that it
// names them: a status the code writes without an edge here, or a state added
// to Gardener's list without one, fails the edge-set test. deleted is final;
// the sweep skips a cluster once its deletion is confirmed.
var shootLifecycle = machine.Must(
	machine.New("shoot lifecycle", shootUnchecked, shootLifecycleEdges()...).
		Terminal(gardener.StatusDeleted),
)

// shootStatuses are the values the clusters_ck_shoot_status constraint allows,
// which the machine and the database must agree on. A status added to one
// without the other fails the edge-set test.
var shootStatuses = []gardener.ShootStatusType{
	gardener.StatusPending,
	gardener.StatusProgressing,
	gardener.StatusReady,
	gardener.StatusError,
	gardener.StatusDeleting,
	gardener.StatusDeleted,
	gardener.StatusLost,
}

func shootLifecycleEdges() []machine.Transition[gardener.ShootStatusType, gardener.ShootStatusType] {
	from := make([]gardener.ShootStatusType, 0, len(shootStatuses))
	from = append(from, shootUnchecked)
	for _, status := range shootStatuses {
		if status != gardener.StatusDeleted {
			from = append(from, status)
		}
	}
	edges := make([]machine.Transition[gardener.ShootStatusType, gardener.ShootStatusType], 0, len(from)*len(shootStatuses))
	for _, f := range from {
		for _, t := range shootStatuses {
			edges = append(edges, machine.Transition[gardener.ShootStatusType, gardener.ShootStatusType]{From: f, Event: t, To: t})
		}
	}
	return edges
}

// shootTransitionDeclared reports whether a check may move a cluster from one
// status to another.
func shootTransitionDeclared(from, to gardener.ShootStatusType) bool {
	_, ok := shootLifecycle.Apply(from, to)
	return ok
}
