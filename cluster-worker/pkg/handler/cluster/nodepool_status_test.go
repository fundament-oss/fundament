package cluster

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/common/dbconst"
	"github.com/fundament-oss/fundament/common/kubename"
)

// Gardener names machine deployments after the generated worker name, so the
// fixtures must carry kubename.GenerateWorkerName(pool), never the raw pool
// name — production text never contains the raw name.
// fixtureTechnicalID is the technical ID the fixtures' names start with.
const fixtureTechnicalID = "shoot--acme--tf-a"

func enrMessage(pool string) string {
	return `machine deployment "` + fixtureTechnicalID + `-` + kubename.GenerateWorkerName(pool) + `-z1" is waiting for machines to join`
}

func TestNextNodePoolStatuses(t *testing.T) {
	t.Parallel()

	enrNomach := enrMessage("nomach")
	stuck := &gardener.ShootStatus{
		Status: gardener.StatusProgressing, Operation: gardener.OperationReconcile,
		Message:               "Reconcile: Waiting until worker resource status is updated",
		EveryNodeReadyMessage: enrNomach,
		TechnicalID:           fixtureTechnicalID,
	}
	quietReconcile := &gardener.ShootStatus{
		Status: gardener.StatusProgressing, Operation: gardener.OperationReconcile,
		Message: "Reconcile: 10% done",
	}
	readyHealthy := &gardener.ShootStatus{Status: gardener.StatusReady, Healthy: true}

	tests := []struct {
		name        string
		stored      []storedNodePool
		observed    *gardener.ShootStatus
		updating    bool
		syncPending bool
		want        []nodePoolUpdate
	}{
		{
			name:     "named pool waits; settled pool keeps its status; new pool progresses",
			stored:   []storedNodePool{{Name: "extra", Status: "ready"}, {Name: "fresh"}, {Name: "nomach", Status: "progressing"}},
			observed: stuck,
			updating: true,
			want: []nodePoolUpdate{
				{Name: "fresh", Status: dbconst.NodePoolStatus_Progressing},
				{Name: "nomach", Status: dbconst.NodePoolStatus_Waiting, Message: enrNomach,
					Events: []statusEvent{{Type: dbconst.ClusterEventEventType_NodepoolWaiting, Message: "Node pool nomach: " + enrNomach}}},
			},
		},
		{
			name:     "a node problem in a settled pool is left to its metrics",
			stored:   []storedNodePool{{Name: "nomach", Status: "ready"}},
			observed: stuck,
			want:     nil,
		},
		{
			name:     "a node problem in a pool from before this feature is left to its metrics",
			stored:   []storedNodePool{{Name: "nomach"}},
			observed: stuck,
			want:     nil,
		},
		{
			name:   "the initial create builds every pool",
			stored: []storedNodePool{{Name: "workers"}, {Name: "gpu"}},
			observed: &gardener.ShootStatus{
				Status: gardener.StatusProgressing, Operation: gardener.OperationCreate,
				Message: "Create: Waiting until worker resource status is updated",
			},
			want: []nodePoolUpdate{
				{Name: "workers", Status: dbconst.NodePoolStatus_Progressing},
				{Name: "gpu", Status: dbconst.NodePoolStatus_Progressing},
			},
		},
		{
			name:   "a pool stuck during the initial create reports waiting",
			stored: []storedNodePool{{Name: "nomach"}},
			observed: &gardener.ShootStatus{
				Status: gardener.StatusProgressing, Operation: gardener.OperationCreate,
				EveryNodeReadyMessage: enrNomach,
				TechnicalID:           fixtureTechnicalID,
			},
			want: []nodePoolUpdate{
				{Name: "nomach", Status: dbconst.NodePoolStatus_Waiting, Message: enrNomach,
					Events: []statusEvent{{Type: dbconst.ClusterEventEventType_NodepoolWaiting, Message: "Node pool nomach: " + enrNomach}}},
			},
		},
		{
			name:     "a scale-up that cannot get machines turns a settled pool waiting",
			stored:   []storedNodePool{{Name: "nomach", Status: "ready"}},
			observed: stuck,
			updating: true,
			want: []nodePoolUpdate{
				{Name: "nomach", Status: dbconst.NodePoolStatus_Waiting, Message: enrNomach,
					Events: []statusEvent{{Type: dbconst.ClusterEventEventType_NodepoolWaiting, Message: "Node pool nomach: " + enrNomach}}},
			},
		},
		{
			name:     "raw pool name in the text does not match",
			stored:   []storedNodePool{{Name: "nomach", Status: "ready"}},
			observed: &gardener.ShootStatus{Status: gardener.StatusProgressing, Operation: gardener.OperationReconcile, EveryNodeReadyMessage: `machine deployment "shoot--acme--tf-a-nomach-z1" is waiting`, TechnicalID: fixtureTechnicalID},
			want:     nil,
		},
		{
			name:     "routine reconcile leaves settled pools alone",
			stored:   []storedNodePool{{Name: "web", Status: "ready"}, {Name: "db", Status: "ready"}},
			observed: quietReconcile,
			want:     nil,
		},
		{
			name:     "a rollout meeting unclassified pools mid-reconcile stays silent",
			stored:   []storedNodePool{{Name: "legacy"}},
			observed: quietReconcile,
			want:     nil,
		},
		{
			name:        "a new pool is progressing while its change is still being applied",
			stored:      []storedNodePool{{Name: "fresh"}},
			observed:    readyHealthy,
			syncPending: true,
			want:        []nodePoolUpdate{{Name: "fresh", Status: dbconst.NodePoolStatus_Progressing}},
		},
		{
			name:        "a ready shoot says nothing about a pool whose change never reached Gardener",
			stored:      []storedNodePool{{Name: "big", Status: "progressing"}, {Name: "web", Status: "ready"}},
			observed:    readyHealthy,
			syncPending: true,
			want:        nil,
		},
		{
			name:     "ready after the rollout settles the pool",
			stored:   []storedNodePool{{Name: "big", Status: "progressing"}},
			observed: readyHealthy,
			updating: true,
			want: []nodePoolUpdate{
				{Name: "big", Status: dbconst.NodePoolStatus_Ready,
					Events: []statusEvent{{Type: dbconst.ClusterEventEventType_NodepoolReady, Message: "Node pool big: machines are ready"}}},
			},
		},
		{
			name:     "first classification of a settled pool is silent",
			stored:   []storedNodePool{{Name: "extra", Status: ""}},
			observed: readyHealthy,
			want:     []nodePoolUpdate{{Name: "extra", Status: dbconst.NodePoolStatus_Ready}},
		},
		{
			name:   "a stuck pool stays stuck while the detail names another pool",
			stored: []storedNodePool{{Name: "tfpool", Status: "waiting", Message: enrMessage("tfpool")}, {Name: "nomach", Status: "waiting", Message: enrNomach}},
			observed: &gardener.ShootStatus{
				Status: gardener.StatusProgressing, Operation: gardener.OperationReconcile,
				// EveryNodeReady names only the first failing machine deployment;
				// with two stuck pools the named one must not flip the other back.
				EveryNodeReadyMessage: enrNomach,
				TechnicalID:           fixtureTechnicalID,
			},
			want: nil,
		},
		{
			name:   "a ready shoot still carrying worker detail settles no stuck pool",
			stored: []storedNodePool{{Name: "tfpool", Status: "waiting", Message: enrMessage("tfpool")}, {Name: "nomach", Status: "waiting", Message: enrNomach}},
			observed: &gardener.ShootStatus{
				// The shoot turned ready while unhealthy: the detail names only
				// nomach, which says nothing about tfpool's machines.
				Status: gardener.StatusReady, Healthy: false,
				EveryNodeReadyMessage: enrNomach,
				TechnicalID:           fixtureTechnicalID,
			},
			want: nil,
		},
		{
			name:   "a recreated pool does not inherit its predecessor's complaint",
			stored: []storedNodePool{{Name: "nomach"}},
			observed: &gardener.ShootStatus{
				// The old pool of the same name was stuck; the new pool's change
				// has not left the outbox, so this report predates it.
				Status: gardener.StatusReady, Healthy: false,
				EveryNodeReadyMessage: enrNomach,
				TechnicalID:           fixtureTechnicalID,
			},
			syncPending: true,
			want:        []nodePoolUpdate{{Name: "nomach", Status: dbconst.NodePoolStatus_Progressing}},
		},
		{
			name:     "a stuck pool keeps its reason while the report carries no detail",
			stored:   []storedNodePool{{Name: "nomach", Status: "waiting", Message: enrNomach}},
			observed: quietReconcile,
			want:     nil,
		},
		{
			name:     "a changed reason refreshes the row without a second event",
			stored:   []storedNodePool{{Name: "nomach", Status: "waiting", Message: "old reason naming " + kubename.GenerateWorkerName("nomach")}},
			observed: stuck,
			want: []nodePoolUpdate{
				{Name: "nomach", Status: dbconst.NodePoolStatus_Waiting, Message: enrNomach},
			},
		},
		{
			name:     "recovery records nodepool_ready",
			stored:   []storedNodePool{{Name: "nomach", Status: "waiting", Message: enrNomach}},
			observed: readyHealthy,
			want: []nodePoolUpdate{
				{Name: "nomach", Status: dbconst.NodePoolStatus_Ready,
					Events: []statusEvent{{Type: dbconst.ClusterEventEventType_NodepoolReady, Message: "Node pool nomach: machines are ready"}}},
			},
		},
		{
			name:   "a pool never matches another pool's worker name",
			stored: []storedNodePool{{Name: "web", Status: "ready"}, {Name: "my-web", Status: "ready"}},
			observed: &gardener.ShootStatus{
				Status: gardener.StatusProgressing, Operation: gardener.OperationReconcile,
				EveryNodeReadyMessage: enrMessage("my-web"),
				TechnicalID:           fixtureTechnicalID,
			},
			updating: true,
			want: []nodePoolUpdate{
				{Name: "my-web", Status: dbconst.NodePoolStatus_Waiting,
					Message: enrMessage("my-web"),
					Events: []statusEvent{{Type: dbconst.ClusterEventEventType_NodepoolWaiting,
						Message: "Node pool my-web: " + enrMessage("my-web")}}},
			},
		},
		{
			name:     "non-pool unhealthiness leaves pools alone",
			stored:   []storedNodePool{{Name: "extra", Status: "ready"}},
			observed: &gardener.ShootStatus{Status: gardener.StatusReady, Healthy: false, Message: gardener.MsgShootUnhealthy},
			want:     nil,
		},
		{
			name:   "failed operation naming the pool records nodepool_error",
			stored: []storedNodePool{{Name: "nomach", Status: "waiting", Message: "old"}},
			observed: &gardener.ShootStatus{Status: gardener.StatusError,
				Message:    "Failed to reconcile",
				LastErrors: []string{enrMessage("nomach")}, TechnicalID: fixtureTechnicalID},
			want: []nodePoolUpdate{
				{Name: "nomach", Status: dbconst.NodePoolStatus_Error,
					Message: enrMessage("nomach"),
					Events: []statusEvent{{Type: dbconst.ClusterEventEventType_NodepoolError,
						Message: "Node pool nomach: " + enrMessage("nomach")}}},
			},
		},
		{
			name:     "status flips waiting to error with identical message still records nodepool_error",
			stored:   []storedNodePool{{Name: "nomach", Status: "waiting", Message: enrNomach}},
			observed: &gardener.ShootStatus{Status: gardener.StatusError, Message: "Failed to reconcile", LastErrors: []string{enrNomach}, TechnicalID: fixtureTechnicalID},
			want: []nodePoolUpdate{
				{Name: "nomach", Status: dbconst.NodePoolStatus_Error, Message: enrNomach,
					Events: []statusEvent{{Type: dbconst.ClusterEventEventType_NodepoolError, Message: "Node pool nomach: " + enrNomach}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, nextNodePoolStatuses(tt.stored, tt.observed, tt.updating, tt.syncPending))
		})
	}
}

// Gardener's reason is node and provider text: unbounded, and it can carry
// terminal control codes. It is cleaned before it is stored or shown.
func TestReasonText(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "plain reason", reasonText("plain reason"))
	assert.Equal(t, "two  lines and a tab", reasonText("two \nlines\x1b and\a a\ttab"))

	long := strings.Repeat("x", 2*maxReasonLen)
	cleaned := reasonText(long)
	assert.Equal(t, maxReasonLen, len([]rune(cleaned)))
	assert.Equal(t, "…", string([]rune(cleaned)[maxReasonLen-1]))
}

// The waiting reason a pool stores went through reasonText.
func TestPoolComplaintSanitizes(t *testing.T) {
	t.Parallel()

	observed := &gardener.ShootStatus{
		Status: gardener.StatusProgressing, Operation: gardener.OperationReconcile,
		EveryNodeReadyMessage: enrMessage("nomach") + "\x1b\x07\nsecond line",
		TechnicalID:           fixtureTechnicalID,
	}
	assert.Equal(t, enrMessage("nomach")+" second line", poolComplaint("nomach", observed))
}

// metalTechnicalID and metalMessage are the poc's EveryNodeReady text from a
// shoot on metal-stack whose pool got no machines (issue 1383): machine names
// are <technicalID>-<worker>-<hash>-<id>, with no zone suffix.
const metalTechnicalID = "shoot--systemd819--enricotef4f"

func metalMessage(pool string) string {
	return `Worker extension (` + metalTechnicalID + `/enricotef4f) reports failing health check: machine "` + metalTechnicalID + `-` + kubename.GenerateWorkerName(pool) + `-68c79-bnnvm" failed: Cloud provider message - machine codes error: code = [Internal] message = [[POST /v1/machine/allocate][422] allocateMachine default "{\"statuscode\":422,\"message\":\"no machine available\"}"].`
}

func TestPoolComplaintMetalNames(t *testing.T) {
	t.Parallel()

	t.Run("EveryNodeReady names the pool", func(t *testing.T) {
		t.Parallel()
		observed := &gardener.ShootStatus{EveryNodeReadyMessage: metalMessage("medium"), TechnicalID: metalTechnicalID}
		assert.Equal(t, metalMessage("medium"), poolComplaint("medium", observed))
		assert.Empty(t, poolComplaint("small", observed))
	})

	// No lastErrors sample from metal-stack exists yet; this wraps the same
	// machine name in a task error the way Gardener reports a failed task.
	t.Run("lastErrors name the pool", func(t *testing.T) {
		t.Parallel()
		medium := "task \"Waiting until shoot worker nodes have been reconciled\" failed: " + metalMessage("medium")
		observed := &gardener.ShootStatus{
			Status:      gardener.StatusError,
			LastErrors:  []string{"task \"Deploying Shoot control plane\" failed: timeout", medium},
			TechnicalID: metalTechnicalID,
		}
		assert.Equal(t, medium, poolComplaint("medium", observed))
	})
}

func TestPoolComplaintMatchesWholeNames(t *testing.T) {
	t.Parallel()

	worker := kubename.GenerateWorkerName("nomach")
	tests := []struct {
		name        string
		text        string
		technicalID string
		want        bool
	}{
		{"local machine deployment", `machine deployment "shoot--acme--tf-a-` + worker + `-z1" is waiting`, "shoot--acme--tf-a", true},
		{"name at the end of the text", "machine shoot--acme--tf-a-" + worker, "shoot--acme--tf-a", true},
		{"another shoot's machine with the same worker name", `machine "shoot--other--tf-b-` + worker + `-68c79-bnnvm" failed`, "shoot--acme--tf-a", false},
		{"a longer name that starts like the worker", `machine "shoot--acme--tf-a-` + worker + `x-z1" failed`, "shoot--acme--tf-a", false},
		{"the worker name without the technical ID", `machine "` + worker + `-z1" failed`, "shoot--acme--tf-a", false},
		{"a later occurrence is a whole name", "shoot--acme--tf-a-" + worker + "x and shoot--acme--tf-a-" + worker + "-z1", "shoot--acme--tf-a", true},
		{"no technical ID yet", `machine deployment "shoot--acme--tf-a-` + worker + `-z1" is waiting`, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			observed := &gardener.ShootStatus{EveryNodeReadyMessage: tt.text, TechnicalID: tt.technicalID}
			assert.Equal(t, tt.want, poolComplaint("nomach", observed) != "")
		})
	}
}
