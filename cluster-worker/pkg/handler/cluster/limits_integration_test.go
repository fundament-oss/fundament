package cluster_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	namespacehandler "github.com/fundament-oss/fundament/cluster-worker/pkg/handler/namespace"
	"github.com/fundament-oss/fundament/common/kubename"
)

// insertOrg inserts a fresh organization so limit-trigger fan-out counts are
// fully determined by the test's own clusters/namespaces (the shared acme
// testdata org would make the expected row counts depend on testdata).
func insertOrg(t *testing.T, db *testDB, name string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := db.adminPool.QueryRow(t.Context(),
		`INSERT INTO tenant.organizations (name, alias) VALUES ($1, $1) RETURNING id`,
		name,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

// outboxCounts returns the number of cluster_outbox rows carrying a cluster_id
// and a namespace_id respectively, restricted to trigger-sourced 'updated'
// events — the only shape the limit triggers may produce. Tests snapshot these
// after setup and assert exact deltas.
func outboxCounts(t *testing.T, db *testDB) (clusterRows, namespaceRows int) {
	t.Helper()
	err := db.adminPool.QueryRow(t.Context(),
		`SELECT
		     count(*) FILTER (WHERE cluster_id IS NOT NULL),
		     count(*) FILTER (WHERE namespace_id IS NOT NULL)
		 FROM tenant.cluster_outbox
		 WHERE event = 'updated' AND source = 'trigger'`,
	).Scan(&clusterRows, &namespaceRows)
	require.NoError(t, err)
	return clusterRows, namespaceRows
}

// Task 1.9: a default_* change on organization_limits enqueues one namespace_id
// row per active namespace across the org's projects, and no cluster_id row.
func TestOrgLimitsTrigger_DefaultChangeEnqueuesNamespaces(t *testing.T) {
	db := createTestDB(t)
	orgID := insertOrg(t, db, "limits-org-defaults")
	clusterA := insertCluster(t, db, orgID, "limits-defaults-a")
	clusterB := insertCluster(t, db, orgID, "limits-defaults-b")
	projectA := insertProject(t, db, clusterA, "limits-proj-a")
	projectB := insertProject(t, db, clusterB, "limits-proj-b")
	insertNamespace(t, db, projectA, "team-a")
	insertNamespace(t, db, projectA, "team-b")
	insertNamespace(t, db, projectB, "team-c")
	deletedNS := insertNamespace(t, db, projectB, "team-gone")
	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.namespaces SET deleted = now() WHERE id = $1`, deletedNS)
	require.NoError(t, err)

	// INSERT with no defaults set enqueues nothing.
	clustersBefore, namespacesBefore := outboxCounts(t, db)
	_, err = db.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.organization_limits (organization_id) VALUES ($1)`, orgID)
	require.NoError(t, err)
	clustersAfter, namespacesAfter := outboxCounts(t, db)
	require.Equal(t, clustersBefore, clustersAfter, "all-NULL insert must not enqueue clusters")
	require.Equal(t, namespacesBefore, namespacesAfter, "all-NULL insert must not enqueue namespaces")

	// Setting a default enqueues the org's active namespaces only.
	_, err = db.adminPool.Exec(t.Context(),
		`UPDATE tenant.organization_limits SET default_cpu_request_m = 100
		 WHERE organization_id = $1 AND deleted IS NULL`, orgID)
	require.NoError(t, err)

	clustersFinal, namespacesFinal := outboxCounts(t, db)
	require.Equal(t, clustersAfter, clustersFinal, "default change must not enqueue clusters")
	require.Equal(t, namespacesAfter+3, namespacesFinal, "one row per active namespace across the org")
}

// Task 1.10: a default_* change on project_limits enqueues one namespace_id row
// per active namespace in that project only.
func TestProjectLimitsTrigger_DefaultChangeEnqueuesNamespaces(t *testing.T) {
	db := createTestDB(t)
	orgID := insertOrg(t, db, "limits-org-project")
	clusterID := insertCluster(t, db, orgID, "limits-project-c")
	projectA := insertProject(t, db, clusterID, "limits-proj-target")
	projectB := insertProject(t, db, clusterID, "limits-proj-other")
	nsA1 := insertNamespace(t, db, projectA, "team-a")
	nsA2 := insertNamespace(t, db, projectA, "team-b")
	insertNamespace(t, db, projectB, "team-other")

	clustersBefore, namespacesBefore := outboxCounts(t, db)

	_, err := db.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.project_limits (project_id, default_memory_limit_mi)
		 VALUES ($1, 512)`, projectA)
	require.NoError(t, err)

	clustersAfter, namespacesAfter := outboxCounts(t, db)
	require.Equal(t, clustersBefore, clustersAfter)
	require.Equal(t, namespacesBefore+2, namespacesAfter, "only the target project's namespaces")

	var enqueued int
	err = db.adminPool.QueryRow(t.Context(),
		`SELECT count(*) FROM tenant.cluster_outbox
		 WHERE namespace_id = ANY($1) AND event = 'updated' AND source = 'trigger'`,
		[]uuid.UUID{nsA1, nsA2},
	).Scan(&enqueued)
	require.NoError(t, err)
	require.Equal(t, 2, enqueued, "the new rows reference the target project's namespaces")
}

// Task 1.11: soft-deleting an organization_limits or project_limits row that
// carried defaults enqueues the affected namespaces, and never a cluster.
func TestLimitsTrigger_SoftDeleteEnqueues(t *testing.T) {
	db := createTestDB(t)
	orgID := insertOrg(t, db, "limits-org-softdelete")
	clusterID := insertCluster(t, db, orgID, "limits-softdelete-c")
	projectID := insertProject(t, db, clusterID, "limits-proj-sd")
	insertNamespace(t, db, projectID, "team-a")

	_, err := db.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.organization_limits (organization_id, default_cpu_limit_m)
		 VALUES ($1, 500)`, orgID)
	require.NoError(t, err)
	_, err = db.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.project_limits (project_id, default_memory_request_mi)
		 VALUES ($1, 256)`, projectID)
	require.NoError(t, err)

	clustersBefore, namespacesBefore := outboxCounts(t, db)

	_, err = db.adminPool.Exec(t.Context(),
		`UPDATE tenant.organization_limits SET deleted = now()
		 WHERE organization_id = $1 AND deleted IS NULL`, orgID)
	require.NoError(t, err)

	clustersAfter, namespacesAfter := outboxCounts(t, db)
	require.Equal(t, clustersBefore, clustersAfter, "org limits soft-delete must not enqueue clusters")
	require.Equal(t, namespacesBefore+1, namespacesAfter, "org limits soft-delete re-syncs namespaces")

	_, err = db.adminPool.Exec(t.Context(),
		`UPDATE tenant.project_limits SET deleted = now()
		 WHERE project_id = $1 AND deleted IS NULL`, projectID)
	require.NoError(t, err)

	clustersFinal, namespacesFinal := outboxCounts(t, db)
	require.Equal(t, clustersAfter, clustersFinal, "project limits soft-delete must not enqueue clusters")
	require.Equal(t, namespacesAfter+1, namespacesFinal, "project limits soft-delete re-syncs namespaces")
}

// Tasks 2.2/6.6 end-to-end: the namespace sync reads the limits tables through
// the fun_cluster_worker role (exercising the RLS read policies and grants),
// merges org and project defaults lowest-wins, and materializes/clears the
// managed LimitRange on the shoot.
func TestNamespaceSync_LimitRangeFromMergedDefaults(t *testing.T) {
	db := createTestDB(t)
	mock := newMockShoot(t)
	h := newNamespaceHandler(t, db, mock)
	ctx := t.Context()

	clusterID := insertCluster(t, db, acmeCorpOrgID, "ns-limits-e2e")
	setShootStatus(t, db, clusterID, "ready")
	projectID := insertProject(t, db, clusterID, "proj-limits")
	nsID := insertNamespace(t, db, projectID, "team-a")
	clusterNS := kubename.GenerateNamespace("proj-limits", "team-a")

	_, err := db.adminPool.Exec(ctx,
		`INSERT INTO tenant.organization_limits (organization_id, default_cpu_request_m, default_cpu_limit_m, default_memory_limit_mi)
		 VALUES ($1, 100, 500, 512)`, acmeCorpOrgID)
	require.NoError(t, err)
	_, err = db.adminPool.Exec(ctx,
		`INSERT INTO tenant.project_limits (project_id, default_cpu_limit_m)
		 VALUES ($1, 250)`, projectID)
	require.NoError(t, err)

	require.NoError(t, h.Sync(ctx, nsID, nsSyncCtx))

	lr := mock.GetLimitRange(clusterID, clusterNS)
	require.NotNil(t, lr, "managed LimitRange must be applied")
	require.NotNil(t, lr.Defaults.CPURequestMilli)
	require.EqualValues(t, 100, *lr.Defaults.CPURequestMilli, "org request applies")
	require.NotNil(t, lr.Defaults.CPULimitMilli)
	require.EqualValues(t, 250, *lr.Defaults.CPULimitMilli, "lower project limit wins")
	require.NotNil(t, lr.Defaults.MemoryLimitMi)
	require.EqualValues(t, 512, *lr.Defaults.MemoryLimitMi)
	require.Nil(t, lr.Defaults.MemoryRequestMi, "unset field stays absent")
	require.Equal(t, namespacehandler.ManagedByValue, lr.Labels[namespacehandler.LabelManagedBy])

	// Clearing all defaults removes the managed LimitRange on the next sync.
	_, err = db.adminPool.Exec(ctx, `UPDATE tenant.organization_limits SET deleted = now() WHERE organization_id = $1`, acmeCorpOrgID)
	require.NoError(t, err)
	_, err = db.adminPool.Exec(ctx, `UPDATE tenant.project_limits SET deleted = now() WHERE project_id = $1`, projectID)
	require.NoError(t, err)

	require.NoError(t, h.Sync(ctx, nsID, nsSyncCtx))
	require.Nil(t, mock.GetLimitRange(clusterID, clusterNS), "cleared defaults must remove the LimitRange")
}

// Task 1.12: a limit change affecting zero active namespaces inserts no rows
// and does not error.
func TestLimitsTrigger_NoActiveTargetsNoop(t *testing.T) {
	db := createTestDB(t)
	orgID := insertOrg(t, db, "limits-org-empty")

	clustersBefore, namespacesBefore := outboxCounts(t, db)

	_, err := db.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.organization_limits (organization_id, default_cpu_limit_m)
		 VALUES ($1, 500)`, orgID)
	require.NoError(t, err)

	clustersAfter, namespacesAfter := outboxCounts(t, db)
	require.Equal(t, clustersBefore, clustersAfter, "a limits change must never enqueue clusters")
	require.Equal(t, namespacesBefore, namespacesAfter)
}
