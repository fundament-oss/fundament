package cluster_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	namespacehandler "github.com/fundament-oss/fundament/cluster-worker/pkg/handler/namespace"
	"github.com/fundament-oss/fundament/common/kubename"
)

// insertOrg inserts a fresh organization so the trigger fan-out counts are
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
// events — the only shape the defaults triggers may produce. Tests snapshot
// these after setup and assert exact deltas.
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

// A cluster default change enqueues one namespace row per active namespace on
// the cluster, skipping soft-deleted namespaces and soft-deleted projects, and
// never touches another cluster's.
func TestClusterDefaultsTrigger_EnqueuesClusterNamespaces(t *testing.T) {
	db := createTestDB(t)
	orgID := insertOrg(t, db, "defaults-org-cluster")
	clusterA := insertCluster(t, db, orgID, "defaults-cluster-a")
	clusterB := insertCluster(t, db, orgID, "defaults-cluster-b")

	projectA1 := insertProject(t, db, clusterA, "defaults-proj-a1")
	projectA2 := insertProject(t, db, clusterA, "defaults-proj-a2")
	projectGone := insertProject(t, db, clusterA, "defaults-proj-gone")
	insertProject(t, db, clusterB, "defaults-proj-b1")

	nsA1 := insertNamespace(t, db, projectA1, "team-a")
	nsA2 := insertNamespace(t, db, projectA2, "team-b")
	deletedNS := insertNamespace(t, db, projectA1, "team-gone")
	orphanNS := insertNamespace(t, db, projectGone, "team-orphan")

	// projects_tr_verify_deleted refuses a project that still has live
	// namespaces, so the namespace goes first.
	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.namespaces SET deleted = now() WHERE id = ANY($1)`,
		[]uuid.UUID{deletedNS, orphanNS})
	require.NoError(t, err)
	_, err = db.adminPool.Exec(t.Context(),
		`UPDATE tenant.projects SET deleted = now() WHERE id = $1`, projectGone)
	require.NoError(t, err)

	clustersBefore, namespacesBefore := outboxCounts(t, db)

	_, err = db.adminPool.Exec(t.Context(),
		`UPDATE tenant.clusters SET default_cpu_request_m = 100 WHERE id = $1`, clusterA)
	require.NoError(t, err)

	clustersAfter, namespacesAfter := outboxCounts(t, db)
	require.Equal(t, clustersBefore, clustersAfter,
		"a defaults change must not re-apply the Shoot")
	require.Equal(t, namespacesBefore+2, namespacesAfter,
		"one row per active namespace of an active project on this cluster")

	var enqueued int
	err = db.adminPool.QueryRow(t.Context(),
		`SELECT count(*) FROM tenant.cluster_outbox
		 WHERE namespace_id = ANY($1) AND event = 'updated' AND source = 'trigger'`,
		[]uuid.UUID{nsA1, nsA2},
	).Scan(&enqueued)
	require.NoError(t, err)
	require.Equal(t, 2, enqueued, "the new rows reference this cluster's namespaces")
}

// A project default change enqueues that project's active namespaces only.
func TestProjectDefaultsTrigger_EnqueuesProjectNamespaces(t *testing.T) {
	db := createTestDB(t)
	orgID := insertOrg(t, db, "defaults-org-project")
	clusterID := insertCluster(t, db, orgID, "defaults-project-c")
	projectA := insertProject(t, db, clusterID, "defaults-proj-target")
	projectB := insertProject(t, db, clusterID, "defaults-proj-other")
	nsA1 := insertNamespace(t, db, projectA, "team-a")
	nsA2 := insertNamespace(t, db, projectA, "team-b")
	insertNamespace(t, db, projectB, "team-other")

	clustersBefore, namespacesBefore := outboxCounts(t, db)

	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.projects SET default_memory_limit_mi = 512 WHERE id = $1`, projectA)
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

// The triggers fire on the default columns only: an unrelated write enqueues
// nothing, and a write that leaves a default at its old value does not either.
func TestDefaultsTrigger_UnrelatedColumnsNoop(t *testing.T) {
	db := createTestDB(t)
	orgID := insertOrg(t, db, "defaults-org-unrelated")
	clusterID := insertCluster(t, db, orgID, "defaults-unrelated-c")
	projectID := insertProject(t, db, clusterID, "defaults-proj-unrelated")
	insertNamespace(t, db, projectID, "team-a")

	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.clusters SET default_cpu_limit_m = 500 WHERE id = $1`, clusterID)
	require.NoError(t, err)

	clustersBefore, namespacesBefore := outboxCounts(t, db)

	_, err = db.adminPool.Exec(t.Context(),
		`UPDATE tenant.clusters SET shoot_status = 'ready' WHERE id = $1`, clusterID)
	require.NoError(t, err)
	_, err = db.adminPool.Exec(t.Context(),
		`UPDATE tenant.projects SET alias = 'renamed' WHERE id = $1`, projectID)
	require.NoError(t, err)
	// Writing the same value is not a change.
	_, err = db.adminPool.Exec(t.Context(),
		`UPDATE tenant.clusters SET default_cpu_limit_m = 500 WHERE id = $1`, clusterID)
	require.NoError(t, err)

	clustersAfter, namespacesAfter := outboxCounts(t, db)
	require.Equal(t, clustersBefore, clustersAfter)
	require.Equal(t, namespacesBefore, namespacesAfter)
}

// A cluster or project with no active namespaces inserts no rows and does not
// error.
func TestDefaultsTrigger_NoActiveTargetsNoop(t *testing.T) {
	db := createTestDB(t)
	orgID := insertOrg(t, db, "defaults-org-empty")
	clusterID := insertCluster(t, db, orgID, "defaults-empty-c")
	projectID := insertProject(t, db, clusterID, "defaults-proj-empty")

	clustersBefore, namespacesBefore := outboxCounts(t, db)

	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.clusters SET default_cpu_limit_m = 500 WHERE id = $1`, clusterID)
	require.NoError(t, err)
	_, err = db.adminPool.Exec(t.Context(),
		`UPDATE tenant.projects SET default_cpu_limit_m = 250 WHERE id = $1`, projectID)
	require.NoError(t, err)

	clustersAfter, namespacesAfter := outboxCounts(t, db)
	require.Equal(t, clustersBefore, clustersAfter)
	require.Equal(t, namespacesBefore, namespacesAfter)
}

// The write-time triggers: a project may not exceed its cluster per field, and
// the effective request may not exceed the effective limit. Exercised here
// against the migrated schema; the API-level mapping lives in organization-api.
func TestDefaultsTrigger_RejectsProjectAboveCluster(t *testing.T) {
	db := createTestDB(t)
	orgID := insertOrg(t, db, "defaults-org-verify")
	clusterID := insertCluster(t, db, orgID, "defaults-verify-c")
	projectID := insertProject(t, db, clusterID, "defaults-proj-verify")

	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.clusters SET default_cpu_limit_m = 500 WHERE id = $1`, clusterID)
	require.NoError(t, err)

	_, err = db.adminPool.Exec(t.Context(),
		`UPDATE tenant.projects SET default_cpu_limit_m = 600 WHERE id = $1`, projectID)
	require.ErrorContains(t, err, "exceeds the cluster's 500m")

	// Equal is fine.
	_, err = db.adminPool.Exec(t.Context(),
		`UPDATE tenant.projects SET default_cpu_limit_m = 500 WHERE id = $1`, projectID)
	require.NoError(t, err)

	// Lowering the cluster below the project names the project to lower first.
	_, err = db.adminPool.Exec(t.Context(),
		`UPDATE tenant.clusters SET default_cpu_limit_m = 400 WHERE id = $1`, clusterID)
	require.ErrorContains(t, err, "defaults-proj-verify")

	// The effective request against the effective limit, with the request set
	// on the project and the limit on the cluster.
	_, err = db.adminPool.Exec(t.Context(),
		`UPDATE tenant.projects SET default_cpu_request_m = 600, default_cpu_limit_m = NULL WHERE id = $1`, projectID)
	require.ErrorContains(t, err, "effective default CPU request 600m exceeds the effective limit 500m")
}

// Tasks 2.2/6.6 end-to-end: the namespace sync reads the default_* columns
// through the fun_cluster_worker role (exercising the RLS read policies and
// grants), takes the lower value per field and materializes/clears the managed
// LimitRange on the shoot.
func TestNamespaceSync_LimitRangeFromClusterAndProject(t *testing.T) {
	db := createTestDB(t)
	mock := newMockShoot(t)
	h := newNamespaceHandler(t, db, mock)
	ctx := t.Context()

	clusterID := insertCluster(t, db, acmeCorpOrgID, "ns-defaults-e2e")
	setShootStatus(t, db, clusterID, "ready")
	projectID := insertProject(t, db, clusterID, "proj-defaults")
	nsID := insertNamespace(t, db, projectID, "team-a")
	clusterNS := kubename.GenerateNamespace("proj-defaults", "team-a")

	_, err := db.adminPool.Exec(ctx,
		`UPDATE tenant.clusters
		 SET default_cpu_request_m = 100, default_cpu_limit_m = 500, default_memory_limit_mi = 512
		 WHERE id = $1`, clusterID)
	require.NoError(t, err)
	_, err = db.adminPool.Exec(ctx,
		`UPDATE tenant.projects SET default_cpu_limit_m = 250 WHERE id = $1`, projectID)
	require.NoError(t, err)

	require.NoError(t, h.Sync(ctx, nsID, nsSyncCtx))

	lr := mock.GetLimitRange(clusterID, clusterNS)
	require.NotNil(t, lr, "managed LimitRange must be applied")
	require.NotNil(t, lr.Defaults.CPURequestMilli)
	require.EqualValues(t, 100, *lr.Defaults.CPURequestMilli, "inherited from the cluster")
	require.NotNil(t, lr.Defaults.CPULimitMilli)
	require.EqualValues(t, 250, *lr.Defaults.CPULimitMilli, "the project's own, lower value")
	require.NotNil(t, lr.Defaults.MemoryLimitMi)
	require.EqualValues(t, 512, *lr.Defaults.MemoryLimitMi)
	require.Nil(t, lr.Defaults.MemoryRequestMi, "unset on both stays absent")
	require.Equal(t, namespacehandler.ManagedByValue, lr.Labels[namespacehandler.LabelManagedBy])

	// Clearing every default removes the managed LimitRange on the next sync.
	// The project goes first: clearing the cluster's while the project still
	// holds a value is refused.
	_, err = db.adminPool.Exec(ctx,
		`UPDATE tenant.projects SET default_cpu_limit_m = NULL WHERE id = $1`, projectID)
	require.NoError(t, err)
	_, err = db.adminPool.Exec(ctx,
		`UPDATE tenant.clusters
		 SET default_cpu_request_m = NULL, default_cpu_limit_m = NULL, default_memory_limit_mi = NULL
		 WHERE id = $1`, clusterID)
	require.NoError(t, err)

	require.NoError(t, h.Sync(ctx, nsID, nsSyncCtx))
	require.Nil(t, mock.GetLimitRange(clusterID, clusterNS), "cleared defaults must remove the LimitRange")
}
