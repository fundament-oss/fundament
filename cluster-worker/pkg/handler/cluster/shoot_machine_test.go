package cluster

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/common/dbconst"
)

// Every status a check can write must be declared as an edge from the status
// the row held. The matrix crosses every stored state with the reports the
// rules treat differently: a Shoot not created yet or gone, an error Gardener
// retries, a reconcile or migrate of a ready shoot, a finished deletion.
func TestShootLifecycleCoversEveryEmittedUpdate(t *testing.T) {
	t.Parallel()

	stored := []storedShootState{
		{},
		{Status: gardener.StatusPending, Message: "Component \"shoot\"/\"controlplane\" is not yet created"},
		{Status: gardener.StatusProgressing, Message: "Create: Waiting until shoot cluster is reconciled"},
		{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: dbconst.ClusterShootHealth_Healthy},
		{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: dbconst.ClusterShootHealth_Healthy, Updating: true},
		{Status: gardener.StatusReady, Message: gardener.MsgShootAwaitingHealth, Health: dbconst.ClusterShootHealth_Unhealthy},
		{Status: gardener.StatusError, Message: "Reconcile: failed"},
		{Status: gardener.StatusDeleting, Message: "Delete: in progress"},
		{Status: gardener.StatusLost, Message: gardener.MsgShootLost},
	}
	observed := []*gardener.ShootStatus{
		{Status: gardener.StatusPending, Message: "Create: Waiting until shoot is created"},
		{Status: gardener.StatusPending, Message: gardener.MsgShootNotFound},
		{Status: gardener.StatusProgressing, Operation: gardener.OperationCreate, Message: "Create: reconciling"},
		{Status: gardener.StatusProgressing, Operation: gardener.OperationReconcile, Message: "Reconcile: reconciling"},
		{Status: gardener.StatusProgressing, Operation: gardener.OperationMigrate, Message: "Migrate: reconciling"},
		{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Healthy: true},
		{Status: gardener.StatusError, Retrying: true, Message: "Reconcile: retrying"},
		{Status: gardener.StatusError, Message: "Reconcile: failed"},
		{Status: gardener.StatusDeleting, Message: "Delete: in progress"},
		{Status: gardener.StatusDeleted, Message: "Delete: succeeded"},
		{Status: gardener.StatusLost, Message: gardener.MsgShootLost},
	}

	for i := range stored {
		for _, report := range observed {
			update := nextShootStatus(&stored[i], report)
			assert.True(t, shootTransitionDeclared(stored[i].Status, update.Status),
				"stored %q, reported %q: %q is not a declared edge", stored[i].Status, report.Status, update.Status)
		}
	}
}

// A deleted cluster is checked on its own path, which writes deleting while
// the Shoot is still there and deleted once it is gone, from whatever status
// the row held. Only a confirmed deletion ends it.
func TestShootLifecycleDeclaresTheDeletedClusterPath(t *testing.T) {
	t.Parallel()

	for _, from := range []gardener.ShootStatusType{
		shootUnchecked,
		gardener.StatusPending,
		gardener.StatusProgressing,
		gardener.StatusReady,
		gardener.StatusError,
		gardener.StatusDeleting,
		gardener.StatusLost,
	} {
		assert.True(t, shootTransitionDeclared(from, gardener.StatusDeleting), "stored %q to deleting", from)
		assert.True(t, shootTransitionDeclared(from, gardener.StatusDeleted), "stored %q to deleted", from)
	}

	// The sweep skips a cluster once its deletion is confirmed, so nothing
	// moves it on.
	for _, to := range []gardener.ShootStatusType{
		gardener.StatusPending,
		gardener.StatusProgressing,
		gardener.StatusReady,
		gardener.StatusError,
		gardener.StatusDeleting,
		gardener.StatusLost,
	} {
		assert.False(t, shootTransitionDeclared(gardener.StatusDeleted, to), "deleted to %q", to)
	}
}
