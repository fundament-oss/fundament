package projectrbac

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
)

type fakeLister struct {
	rows []db.ProjectRoleBindingListForClusterRow
	err  error
}

func (f *fakeLister) ProjectRoleBindingListForCluster(context.Context, db.ProjectRoleBindingListForClusterParams) ([]db.ProjectRoleBindingListForClusterRow, error) {
	return f.rows, f.err
}

func newConverger(t *testing.T, lister DesiredLister) (*Converger, *shoot.MockShootAccess) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mock := shoot.NewMockShootAccess(logger)
	return NewConverger(lister, mock, logger), mock
}

func TestConverge_CreatesUpdatesAndDeletes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clusterID := uuid.New()
	lister := &fakeLister{rows: []db.ProjectRoleBindingListForClusterRow{
		{UserID: userA, NamespaceID: nsWeb, Role: "viewer"},
		{UserID: userB, NamespaceID: nsWeb, Role: "admin"},
	}}
	c, mock := newConverger(t, lister)
	require.NoError(t, mock.CreateNamespace(ctx, clusterID, "p--web", map[string]string{LabelNamespaceID: nsWeb.String()}))

	require.NoError(t, c.Converge(ctx, clusterID))
	rbA := mock.GetRoleBinding(clusterID, "p--web", BindingName(userA))
	require.NotNil(t, rbA)
	assert.Equal(t, "view", rbA.RoleRef.Name)
	assert.Equal(t, Subjects(userA), rbA.Subjects)
	assert.Equal(t, "admin", mock.GetRoleBinding(clusterID, "p--web", BindingName(userB)).RoleRef.Name)

	// userA promoted, userB removed.
	lister.rows = []db.ProjectRoleBindingListForClusterRow{{UserID: userA, NamespaceID: nsWeb, Role: "admin"}}
	require.NoError(t, c.Converge(ctx, clusterID))
	assert.Equal(t, "admin", mock.GetRoleBinding(clusterID, "p--web", BindingName(userA)).RoleRef.Name)
	assert.Nil(t, mock.GetRoleBinding(clusterID, "p--web", BindingName(userB)))
}

func TestConverge_PropagatesErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clusterID := uuid.New()

	c, _ := newConverger(t, &fakeLister{err: errors.New("db down")})
	require.ErrorContains(t, c.Converge(ctx, clusterID), "db down")

	c, mock := newConverger(t, &fakeLister{rows: []db.ProjectRoleBindingListForClusterRow{{UserID: userA, NamespaceID: nsWeb, Role: "viewer"}}})
	require.NoError(t, mock.CreateNamespace(ctx, clusterID, "p--web", map[string]string{LabelNamespaceID: nsWeb.String()}))
	mock.EnsureRoleBindingError = errors.New("forbidden")
	require.ErrorContains(t, c.Converge(ctx, clusterID), "forbidden")
}

// trackingShoot records how many ListRoleBindings calls run at once per
// cluster, holding each one briefly so overlaps would show.
type trackingShoot struct {
	*shoot.MockShootAccess
	mu       sync.Mutex
	inFlight map[uuid.UUID]int
	maxSeen  map[uuid.UUID]int
	// overlap is closed once calls for two different clusters run at once.
	overlap     chan struct{}
	overlapOnce sync.Once
}

func newTrackingShoot() *trackingShoot {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	return &trackingShoot{
		MockShootAccess: shoot.NewMockShootAccess(logger),
		inFlight:        map[uuid.UUID]int{},
		maxSeen:         map[uuid.UUID]int{},
		overlap:         make(chan struct{}),
	}
}

func (s *trackingShoot) ListRoleBindings(ctx context.Context, clusterID uuid.UUID, labelKey string) ([]shoot.ResourceInfo, error) {
	s.mu.Lock()
	s.inFlight[clusterID]++
	s.maxSeen[clusterID] = max(s.maxSeen[clusterID], s.inFlight[clusterID])
	if len(s.inFlight) > 1 {
		busy := 0
		for _, n := range s.inFlight {
			if n > 0 {
				busy++
			}
		}
		if busy > 1 {
			s.overlapOnce.Do(func() { close(s.overlap) })
		}
	}
	s.mu.Unlock()

	time.Sleep(20 * time.Millisecond)

	s.mu.Lock()
	s.inFlight[clusterID]--
	s.mu.Unlock()
	return s.MockShootAccess.ListRoleBindings(ctx, clusterID, labelKey) //nolint:wrapcheck // test double passes through
}

// usersync and namespace sync each have a Converger, called from the outbox and
// reconcile workers at once: passes on one cluster must not overlap.
func TestConverge_SerializesPerCluster(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	sh := newTrackingShoot()
	lister := &fakeLister{}
	convergers := []*Converger{NewConverger(lister, sh, logger), NewConverger(lister, sh, logger)}
	clusterA, clusterB := uuid.New(), uuid.New()

	var wg sync.WaitGroup
	for i := range 6 {
		for _, cluster := range []uuid.UUID{clusterA, clusterB} {
			wg.Go(func() {
				assert.NoError(t, convergers[i%2].Converge(context.Background(), cluster))
			})
		}
	}
	wg.Wait()

	assert.Equal(t, 1, sh.maxSeen[clusterA], "passes on one cluster overlapped")
	assert.Equal(t, 1, sh.maxSeen[clusterB], "passes on one cluster overlapped")
	overlapped := false
	select {
	case <-sh.overlap:
		overlapped = true
	default:
	}
	assert.True(t, overlapped, "passes on different clusters never ran at the same time")
}

func TestConverge_LockWaitHonoursContext(t *testing.T) {
	t.Parallel()
	clusterID := uuid.New()
	c, _ := newConverger(t, &fakeLister{})

	unlock, err := lockCluster(context.Background(), clusterID)
	require.NoError(t, err)
	defer unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, c.Converge(ctx, clusterID), context.DeadlineExceeded)
}
