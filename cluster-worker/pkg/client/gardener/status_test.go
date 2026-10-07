package gardener

import (
	"io"
	"log/slog"
	"testing"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newStatusTestClient(t *testing.T, shoots ...*gardencorev1beta1.Shoot) *RealClient {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, gardencorev1beta1.AddToScheme(scheme))

	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, shoot := range shoots {
		builder = builder.WithObjects(shoot)
	}

	return &RealClient{
		client: builder.Build(),
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func statusTestShoot(clusterID uuid.UUID, op *gardencorev1beta1.LastOperation, conditions ...gardencorev1beta1.Condition) *gardencorev1beta1.Shoot {
	return &gardencorev1beta1.Shoot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "garden-test",
			Labels:    map[string]string{LabelClusterID: clusterID.String()},
		},
		Status: gardencorev1beta1.ShootStatus{
			LastOperation: op,
			Conditions:    conditions,
		},
	}
}

func lastOp(opType gardencorev1beta1.LastOperationType, state gardencorev1beta1.LastOperationState, description string) *gardencorev1beta1.LastOperation {
	return &gardencorev1beta1.LastOperation{Type: opType, State: state, Description: description}
}

func condition(conditionType gardencorev1beta1.ConditionType, status gardencorev1beta1.ConditionStatus) gardencorev1beta1.Condition {
	return gardencorev1beta1.Condition{Type: conditionType, Status: status}
}

func healthyConditions() []gardencorev1beta1.Condition {
	return []gardencorev1beta1.Condition{
		condition(gardencorev1beta1.ShootAPIServerAvailable, gardencorev1beta1.ConditionTrue),
		condition(gardencorev1beta1.ShootControlPlaneHealthy, gardencorev1beta1.ConditionTrue),
		condition(gardencorev1beta1.ShootSystemComponentsHealthy, gardencorev1beta1.ConditionTrue),
		condition(gardencorev1beta1.ShootEveryNodeReady, gardencorev1beta1.ConditionTrue),
	}
}

// shootWithStatus builds a Shoot carrying the given status, labeled with a
// fresh cluster ID so getStatusFor can look it up.
func shootWithStatus(status gardencorev1beta1.ShootStatus) *gardencorev1beta1.Shoot {
	return &gardencorev1beta1.Shoot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "garden-test",
			Labels:    map[string]string{LabelClusterID: uuid.New().String()},
		},
		Status: status,
	}
}

// getStatusFor builds a RealClient around shoot and returns its computed status.
func getStatusFor(t *testing.T, shoot *gardencorev1beta1.Shoot) *ShootStatus {
	t.Helper()

	clusterID, err := uuid.Parse(shoot.Labels[LabelClusterID])
	require.NoError(t, err)

	r := newStatusTestClient(t, shoot)
	status, err := r.GetShootStatus(t.Context(), &ClusterToSync{ID: clusterID})
	require.NoError(t, err)
	return status
}

func TestRealClientGetShootStatus(t *testing.T) {
	t.Parallel()

	settling := []gardencorev1beta1.Condition{
		condition(gardencorev1beta1.ShootAPIServerAvailable, gardencorev1beta1.ConditionTrue),
		condition(gardencorev1beta1.ShootControlPlaneHealthy, gardencorev1beta1.ConditionProgressing),
		condition(gardencorev1beta1.ShootSystemComponentsHealthy, gardencorev1beta1.ConditionProgressing),
		condition(gardencorev1beta1.ShootEveryNodeReady, gardencorev1beta1.ConditionProgressing),
	}
	missingSystemComponents := []gardencorev1beta1.Condition{
		condition(gardencorev1beta1.ShootAPIServerAvailable, gardencorev1beta1.ConditionTrue),
		condition(gardencorev1beta1.ShootControlPlaneHealthy, gardencorev1beta1.ConditionTrue),
	}

	tests := []struct {
		name       string
		shoot      func(clusterID uuid.UUID) *gardencorev1beta1.Shoot
		wantStatus ShootStatusType
		wantMsg    string
		wantOp     OperationType
		wantHealth bool
		wantRetry  bool
	}{
		{
			name: "create succeeded and all conditions true",
			shoot: func(id uuid.UUID) *gardencorev1beta1.Shoot {
				return statusTestShoot(id, lastOp(gardencorev1beta1.LastOperationTypeCreate, gardencorev1beta1.LastOperationStateSucceeded, "Shoot cluster has been successfully reconciled."), healthyConditions()...)
			},
			wantStatus: StatusReady,
			wantMsg:    MsgShootReady,
			wantOp:     OperationCreate,
			wantHealth: true,
		},
		{
			// Progressing holds under conditionHolds: a routine transition, not a failure.
			name: "create succeeded while conditions still settle",
			shoot: func(id uuid.UUID) *gardencorev1beta1.Shoot {
				return statusTestShoot(id, lastOp(gardencorev1beta1.LastOperationTypeCreate, gardencorev1beta1.LastOperationStateSucceeded, ""), settling...)
			},
			wantStatus: StatusReady,
			wantMsg:    MsgShootReady,
			wantOp:     OperationCreate,
			wantHealth: true,
		},
		{
			name: "required condition missing is unhealthy",
			shoot: func(id uuid.UUID) *gardencorev1beta1.Shoot {
				return statusTestShoot(id, lastOp(gardencorev1beta1.LastOperationTypeReconcile, gardencorev1beta1.LastOperationStateSucceeded, ""), missingSystemComponents...)
			},
			wantStatus: StatusReady,
			wantMsg:    MsgShootUnhealthy,
			wantOp:     OperationReconcile,
		},
		{
			name: "create processing",
			shoot: func(id uuid.UUID) *gardencorev1beta1.Shoot {
				return statusTestShoot(id, lastOp(gardencorev1beta1.LastOperationTypeCreate, gardencorev1beta1.LastOperationStateProcessing, "Waiting for etcd"))
			},
			wantStatus: StatusProgressing,
			wantMsg:    "Create: Waiting for etcd",
			wantOp:     OperationCreate,
		},
		{
			// Progressing branch now also reports Healthy; all conditions True here.
			name: "routine reconcile processing",
			shoot: func(id uuid.UUID) *gardencorev1beta1.Shoot {
				return statusTestShoot(id, lastOp(gardencorev1beta1.LastOperationTypeReconcile, gardencorev1beta1.LastOperationStateProcessing, "Syncing"), healthyConditions()...)
			},
			wantStatus: StatusProgressing,
			wantMsg:    "Reconcile: Syncing",
			wantOp:     OperationReconcile,
			wantHealth: true,
		},
		{
			name: "reconcile pending",
			shoot: func(id uuid.UUID) *gardencorev1beta1.Shoot {
				return statusTestShoot(id, lastOp(gardencorev1beta1.LastOperationTypeReconcile, gardencorev1beta1.LastOperationStatePending, "Queued"))
			},
			wantStatus: StatusProgressing,
			wantMsg:    "Reconcile: Queued",
			wantOp:     OperationReconcile,
		},
		{
			name: "reconcile error",
			shoot: func(id uuid.UUID) *gardencorev1beta1.Shoot {
				return statusTestShoot(id, lastOp(gardencorev1beta1.LastOperationTypeReconcile, gardencorev1beta1.LastOperationStateError, "etcd unavailable"))
			},
			wantStatus: StatusError,
			wantMsg:    "etcd unavailable",
			wantOp:     OperationReconcile,
			wantRetry:  true,
		},
		{
			name: "create failed",
			shoot: func(id uuid.UUID) *gardencorev1beta1.Shoot {
				return statusTestShoot(id, lastOp(gardencorev1beta1.LastOperationTypeCreate, gardencorev1beta1.LastOperationStateFailed, "quota exceeded"))
			},
			wantStatus: StatusError,
			wantMsg:    "quota exceeded",
			wantOp:     OperationCreate,
		},
		{
			name: "operation aborted",
			shoot: func(id uuid.UUID) *gardencorev1beta1.Shoot {
				return statusTestShoot(id, lastOp(gardencorev1beta1.LastOperationTypeReconcile, gardencorev1beta1.LastOperationStateAborted, "stopped"))
			},
			wantStatus: StatusError,
			wantMsg:    "Operation was aborted: stopped",
			wantOp:     OperationReconcile,
			wantRetry:  true,
		},
		{
			name: "no last operation yet",
			shoot: func(id uuid.UUID) *gardencorev1beta1.Shoot {
				return statusTestShoot(id, nil)
			},
			wantStatus: StatusProgressing,
			wantMsg:    "Shoot is being created",
		},
		{
			name: "being deleted",
			shoot: func(id uuid.UUID) *gardencorev1beta1.Shoot {
				shoot := statusTestShoot(id, lastOp(gardencorev1beta1.LastOperationTypeDelete, gardencorev1beta1.LastOperationStateProcessing, ""))
				now := metav1.Now()
				shoot.DeletionTimestamp = &now
				shoot.Finalizers = []string{"gardener"}
				return shoot
			},
			wantStatus: StatusDeleting,
			wantMsg:    "Shoot is being deleted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clusterID := uuid.New()
			r := newStatusTestClient(t, tt.shoot(clusterID))

			got, err := r.GetShootStatus(t.Context(), &ClusterToSync{ID: clusterID})
			require.NoError(t, err)

			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantMsg, got.Message)
			assert.Equal(t, tt.wantOp, got.Operation)
			assert.Equal(t, tt.wantHealth, got.Healthy)
			assert.Equal(t, tt.wantRetry, got.Retrying)
		})
	}
}

func TestRealClientGetShootStatusNotFound(t *testing.T) {
	t.Parallel()

	other := statusTestShoot(uuid.New(), lastOp(gardencorev1beta1.LastOperationTypeCreate, gardencorev1beta1.LastOperationStateSucceeded, ""), healthyConditions()...)
	r := newStatusTestClient(t, other)

	got, err := r.GetShootStatus(t.Context(), &ClusterToSync{ID: uuid.New()})
	require.NoError(t, err)

	assert.Equal(t, StatusPending, got.Status)
	assert.Equal(t, MsgShootNotFound, got.Message)
	assert.Empty(t, got.Operation)
}

func TestGetShootStatusWorkerDetail(t *testing.T) {
	t.Parallel()

	enrMsg := `machine deployment "shoot--acme--tf-a-nomach-z1" is waiting for machines`

	t.Run("progressing reconcile carries condition detail", func(t *testing.T) {
		t.Parallel()
		shoot := shootWithStatus(gardencorev1beta1.ShootStatus{
			LastOperation: &gardencorev1beta1.LastOperation{
				Type:        gardencorev1beta1.LastOperationTypeReconcile,
				State:       gardencorev1beta1.LastOperationStateProcessing,
				Description: "Waiting until worker resource status is updated",
			},
			Conditions: []gardencorev1beta1.Condition{
				{Type: gardencorev1beta1.ShootAPIServerAvailable, Status: gardencorev1beta1.ConditionTrue},
				{Type: gardencorev1beta1.ShootControlPlaneHealthy, Status: gardencorev1beta1.ConditionTrue},
				{Type: gardencorev1beta1.ShootSystemComponentsHealthy, Status: gardencorev1beta1.ConditionTrue},
				{Type: gardencorev1beta1.ShootEveryNodeReady, Status: gardencorev1beta1.ConditionFalse, Message: enrMsg},
			},
			LastErrors: []gardencorev1beta1.LastError{{Description: "no machine available"}},
		})
		status := getStatusFor(t, shoot)
		assert.Equal(t, StatusProgressing, status.Status)
		assert.False(t, status.Healthy, "EveryNodeReady False must count as unhealthy")
		assert.Equal(t, enrMsg, status.EveryNodeReadyMessage)
		assert.Equal(t, []string{"no machine available"}, status.LastErrors)
	})

	t.Run("progressing condition does not count as unhealthy", func(t *testing.T) {
		t.Parallel()
		// Same shoot but EveryNodeReady = Progressing (a transition, not a failure).
		shoot := shootWithStatus(gardencorev1beta1.ShootStatus{
			LastOperation: &gardencorev1beta1.LastOperation{
				Type:        gardencorev1beta1.LastOperationTypeReconcile,
				State:       gardencorev1beta1.LastOperationStateProcessing,
				Description: "Waiting until worker resource status is updated",
			},
			Conditions: []gardencorev1beta1.Condition{
				{Type: gardencorev1beta1.ShootAPIServerAvailable, Status: gardencorev1beta1.ConditionTrue},
				{Type: gardencorev1beta1.ShootControlPlaneHealthy, Status: gardencorev1beta1.ConditionTrue},
				{Type: gardencorev1beta1.ShootSystemComponentsHealthy, Status: gardencorev1beta1.ConditionTrue},
				{Type: gardencorev1beta1.ShootEveryNodeReady, Status: gardencorev1beta1.ConditionProgressing},
			},
		})
		status := getStatusFor(t, shoot)
		assert.Equal(t, StatusProgressing, status.Status)
		assert.True(t, status.Healthy, "EveryNodeReady Progressing must not count as unhealthy")
		assert.Empty(t, status.EveryNodeReadyMessage)
		assert.Empty(t, status.LastErrors)
	})

	t.Run("ready and healthy includes EveryNodeReady", func(t *testing.T) {
		t.Parallel()
		// Succeeded op, all four conditions True: StatusReady, Healthy true.
		shoot := shootWithStatus(gardencorev1beta1.ShootStatus{
			LastOperation: &gardencorev1beta1.LastOperation{
				Type:  gardencorev1beta1.LastOperationTypeReconcile,
				State: gardencorev1beta1.LastOperationStateSucceeded,
			},
			Conditions: []gardencorev1beta1.Condition{
				{Type: gardencorev1beta1.ShootAPIServerAvailable, Status: gardencorev1beta1.ConditionTrue},
				{Type: gardencorev1beta1.ShootControlPlaneHealthy, Status: gardencorev1beta1.ConditionTrue},
				{Type: gardencorev1beta1.ShootSystemComponentsHealthy, Status: gardencorev1beta1.ConditionTrue},
				{Type: gardencorev1beta1.ShootEveryNodeReady, Status: gardencorev1beta1.ConditionTrue},
			},
		})
		status := getStatusFor(t, shoot)
		assert.Equal(t, StatusReady, status.Status)
		assert.True(t, status.Healthy)
		assert.Empty(t, status.EveryNodeReadyMessage)
		assert.Empty(t, status.LastErrors)
	})
}
