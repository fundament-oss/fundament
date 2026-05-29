package cluster_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	namespacehandler "github.com/fundament-oss/fundament/cluster-worker/pkg/handler/namespace"
	"github.com/fundament-oss/fundament/common/dbconst"
)

// nsSyncCtx is the SyncContext the outbox worker passes for namespace rows.
// The namespace handler is event-agnostic, so only EntityType matters here.
var nsSyncCtx = handler.SyncContext{EntityType: handler.EntityNamespace}

// insertProject inserts a project for a cluster and returns its id. A project
// requires at least one admin member (enforced by a deferred constraint
// trigger), so this inserts a throwaway user and an admin membership in the
// same statement/transaction.
func insertProject(t *testing.T, db *testDB, clusterID uuid.UUID, name string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := db.adminPool.QueryRow(t.Context(),
		`WITH p AS (
		     INSERT INTO tenant.projects (cluster_id, name) VALUES ($1, $2) RETURNING id
		 ), u AS (
		     INSERT INTO tenant.users (name) VALUES ($2 || '-admin') RETURNING id
		 ), m AS (
		     INSERT INTO tenant.project_members (project_id, user_id, role)
		     SELECT p.id, u.id, 'admin' FROM p, u
		 )
		 SELECT id FROM p`,
		clusterID, name,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

// insertNamespace inserts a namespace for a project and returns its id.
func insertNamespace(t *testing.T, db *testDB, projectID uuid.UUID, name string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := db.adminPool.QueryRow(t.Context(),
		`INSERT INTO tenant.namespaces (project_id, name) VALUES ($1, $2) RETURNING id`,
		projectID, name,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

// latestNamespaceOutbox returns event and source of the most recent outbox row
// for a namespace (empty strings if none).
func latestNamespaceOutbox(t *testing.T, db *testDB, namespaceID uuid.UUID) (event, source string) {
	t.Helper()
	err := db.adminPool.QueryRow(t.Context(),
		`SELECT event, source FROM tenant.cluster_outbox
		 WHERE namespace_id = $1 ORDER BY id DESC LIMIT 1`,
		namespaceID,
	).Scan(&event, &source)
	require.NoError(t, err)
	return event, source
}

func newNamespaceHandler(t *testing.T, db *testDB, mock *shoot.MockShootAccess) *namespacehandler.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	return namespacehandler.New(db.workerPool, mock, 10, logger)
}

func newMockShoot(t *testing.T) *shoot.MockShootAccess {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	return shoot.NewMockShootAccess(logger)
}

// Task 1.11: insert fires the trigger with namespace_id, event=created, source=trigger.
func TestNamespaceTrigger_InsertEnqueues(t *testing.T) {
	db := createTestDB(t)
	clusterID := insertCluster(t, db, acmeCorpOrgID, "ns-trigger-insert")
	projectID := insertProject(t, db, clusterID, "proj-insert")
	nsID := insertNamespace(t, db, projectID, "team-a")

	event, source := latestNamespaceOutbox(t, db, nsID)
	require.Equal(t, "created", event)
	require.Equal(t, "trigger", source)
}

// Task 1.12: soft-delete fires the trigger with event=deleted.
func TestNamespaceTrigger_SoftDeleteEnqueues(t *testing.T) {
	db := createTestDB(t)
	clusterID := insertCluster(t, db, acmeCorpOrgID, "ns-trigger-delete")
	projectID := insertProject(t, db, clusterID, "proj-delete")
	nsID := insertNamespace(t, db, projectID, "team-a")

	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.namespaces SET deleted = now() WHERE id = $1`, nsID)
	require.NoError(t, err)

	event, source := latestNamespaceOutbox(t, db, nsID)
	require.Equal(t, "deleted", event)
	require.Equal(t, "trigger", source)
}

// A name update fires the trigger with event=updated.
func TestNamespaceTrigger_NameUpdateEnqueues(t *testing.T) {
	db := createTestDB(t)
	clusterID := insertCluster(t, db, acmeCorpOrgID, "ns-trigger-rename")
	projectID := insertProject(t, db, clusterID, "proj-rename")
	nsID := insertNamespace(t, db, projectID, "billing")

	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.namespaces SET name = 'billing-v2' WHERE id = $1`, nsID)
	require.NoError(t, err)

	event, source := latestNamespaceOutbox(t, db, nsID)
	require.Equal(t, "updated", event)
	require.Equal(t, "trigger", source)
}

// Task 1.14: ck_single_fk rejects a row with both cluster_id and namespace_id set.
func TestClusterOutbox_SingleFKRejectsClusterPlusNamespace(t *testing.T) {
	db := createTestDB(t)
	clusterID := insertCluster(t, db, acmeCorpOrgID, "ns-single-fk")
	projectID := insertProject(t, db, clusterID, "proj-fk")
	nsID := insertNamespace(t, db, projectID, "team-a")

	_, err := db.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.cluster_outbox (cluster_id, namespace_id, event, source)
		 VALUES ($1, $2, 'created', 'manual')`,
		clusterID, nsID,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cluster_outbox_ck_single_fk")
}

// Task 1.15: a namespace-only outbox row must not update clusters.outbox_status.
func TestNamespaceOutbox_DoesNotUpdateClusterStatus(t *testing.T) {
	db := createTestDB(t)
	clusterID := insertCluster(t, db, acmeCorpOrgID, "ns-fanin")
	projectID := insertProject(t, db, clusterID, "proj-fanin")

	// Drive the cluster's own outbox row to completed so outbox_status is a known value.
	markOutboxCompleted(t, db, clusterID)
	var before *string
	require.NoError(t, db.adminPool.QueryRow(t.Context(),
		`SELECT outbox_status FROM tenant.clusters WHERE id = $1`, clusterID).Scan(&before))
	require.NotNil(t, before)
	require.Equal(t, "completed", *before)

	// Inserting a namespace creates a namespace-only outbox row (status transitions
	// pending). The fan-in trigger must leave clusters.outbox_status untouched.
	insertNamespace(t, db, projectID, "team-a")

	var after *string
	require.NoError(t, db.adminPool.QueryRow(t.Context(),
		`SELECT outbox_status FROM tenant.clusters WHERE id = $1`, clusterID).Scan(&after))
	require.NotNil(t, after)
	require.Equal(t, "completed", *after, "namespace outbox row must not change the cluster's outbox_status")
}

// Task 4.11: full create -> re-sync -> delete cycle through the handler with a real DB.
func TestNamespaceSync_CreateResyncDelete(t *testing.T) {
	db := createTestDB(t)
	mock := newMockShoot(t)
	h := newNamespaceHandler(t, db, mock)
	ctx := context.Background()

	clusterID := insertCluster(t, db, acmeCorpOrgID, "ns-sync-cycle")
	setShootStatus(t, db, clusterID, "ready")
	projectID := insertProject(t, db, clusterID, "proj-cycle")
	nsID := insertNamespace(t, db, projectID, "team-a")

	// Create.
	require.NoError(t, h.Sync(ctx, nsID, nsSyncCtx))
	got, err := mock.GetNamespace(ctx, clusterID, "team-a")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, nsID.String(), got.Labels[namespacehandler.LabelNamespaceID])
	require.Equal(t, clusterID.String(), got.Labels[namespacehandler.LabelClusterID])
	require.Equal(t, namespacehandler.ManagedByValue, got.Labels[namespacehandler.LabelManagedBy])

	// Re-sync is idempotent.
	require.NoError(t, h.Sync(ctx, nsID, nsSyncCtx))

	// Soft-delete -> hard delete on the shoot.
	_, err = db.adminPool.Exec(ctx, `UPDATE tenant.namespaces SET deleted = now() WHERE id = $1`, nsID)
	require.NoError(t, err)
	require.NoError(t, h.Sync(ctx, nsID, nsSyncCtx))
	gone, err := mock.GetNamespace(ctx, clusterID, "team-a")
	require.NoError(t, err)
	require.Nil(t, gone)
}

// Shoot-not-ready defers the row via a PreconditionError.
func TestNamespaceSync_ShootNotReady_Precondition(t *testing.T) {
	db := createTestDB(t)
	mock := newMockShoot(t)
	h := newNamespaceHandler(t, db, mock)
	ctx := context.Background()

	clusterID := insertCluster(t, db, acmeCorpOrgID, "ns-not-ready")
	setShootStatus(t, db, clusterID, "progressing")
	projectID := insertProject(t, db, clusterID, "proj-not-ready")
	nsID := insertNamespace(t, db, projectID, "team-a")

	err := h.Sync(ctx, nsID, nsSyncCtx)
	require.Error(t, err)
	require.ErrorContains(t, err, "precondition not met")

	got, err := mock.GetNamespace(ctx, clusterID, "team-a")
	require.NoError(t, err)
	require.Nil(t, got, "no namespace should be created while the shoot is not ready")
}

// Task 4.12: reconcile enqueues an outbox row for a namespace missing on the shoot.
func TestNamespaceReconcile_EnqueuesMissing(t *testing.T) {
	db := createTestDB(t)
	mock := newMockShoot(t)
	h := newNamespaceHandler(t, db, mock)
	ctx := context.Background()

	clusterID := insertCluster(t, db, acmeCorpOrgID, "ns-reconcile")
	setShootStatus(t, db, clusterID, "ready")
	projectID := insertProject(t, db, clusterID, "proj-reconcile")
	nsID := insertNamespace(t, db, projectID, "team-a")

	// Drain the trigger-created row so reconcile must (re)enqueue.
	_, err := db.adminPool.Exec(ctx,
		`UPDATE tenant.cluster_outbox SET status = 'completed', processed = now() WHERE namespace_id = $1`, nsID)
	require.NoError(t, err)

	require.NoError(t, h.Reconcile(ctx))

	var count int
	require.NoError(t, db.adminPool.QueryRow(ctx,
		`SELECT count(*) FROM tenant.cluster_outbox
		 WHERE namespace_id = $1 AND event = 'reconcile' AND status IN ('pending','retrying')`,
		nsID,
	).Scan(&count))
	require.Equal(t, 1, count, "reconcile should enqueue exactly one row for the missing namespace")
}

// drainNamespaceOutbox marks all outbox rows for a namespace completed, simulating
// rows that were processed (or deferred) before the shoot became ready.
func drainNamespaceOutbox(t *testing.T, db *testDB, nsID uuid.UUID) {
	t.Helper()
	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.cluster_outbox SET status = 'completed', processed = now() WHERE namespace_id = $1`, nsID)
	require.NoError(t, err)
}

// Task 5.3: the namespace handler, on the cluster-ready event, fans out a
// reconcile row for every active namespace on the cluster (same pattern as
// usersync reacting to the ready event).
func TestClusterReadyFanout_EnqueuesActiveNamespaces(t *testing.T) {
	db := createTestDB(t)
	h := newNamespaceHandler(t, db, newMockShoot(t))
	ctx := t.Context()

	clusterID := insertCluster(t, db, acmeCorpOrgID, "ns-ready-fanout")
	projectID := insertProject(t, db, clusterID, "proj-fanout")
	ns1 := insertNamespace(t, db, projectID, "alpha")
	ns2 := insertNamespace(t, db, projectID, "beta")
	ns3 := insertNamespace(t, db, projectID, "gamma")
	// Drain the trigger-created rows so the conditional fan-out must re-enqueue.
	for _, id := range []uuid.UUID{ns1, ns2, ns3} {
		drainNamespaceOutbox(t, db, id)
	}

	readyCtx := handler.SyncContext{EntityType: handler.EntityCluster, Event: dbconst.ClusterOutboxEvent_Ready, Source: dbconst.ClusterOutboxSource_Status}
	require.NoError(t, h.Sync(ctx, clusterID, readyCtx))

	for _, id := range []uuid.UUID{ns1, ns2, ns3} {
		var count int
		require.NoError(t, db.adminPool.QueryRow(ctx,
			`SELECT count(*) FROM tenant.cluster_outbox
			 WHERE namespace_id = $1 AND event = 'reconcile' AND source = 'reconcile'
			   AND status IN ('pending','retrying')`,
			id,
		).Scan(&count))
		require.Equalf(t, 1, count, "expected a fan-out reconcile row for namespace %s", id)
	}
}

// Task 5.4: the ready fan-out is a no-op when the cluster has no namespaces.
func TestClusterReadyFanout_NoNamespacesIsNoop(t *testing.T) {
	db := createTestDB(t)
	h := newNamespaceHandler(t, db, newMockShoot(t))
	ctx := t.Context()

	clusterID := insertCluster(t, db, acmeCorpOrgID, "ns-ready-empty")

	readyCtx := handler.SyncContext{EntityType: handler.EntityCluster, Event: dbconst.ClusterOutboxEvent_Ready, Source: dbconst.ClusterOutboxSource_Status}
	require.NoError(t, h.Sync(ctx, clusterID, readyCtx))

	var count int
	require.NoError(t, db.adminPool.QueryRow(ctx,
		`SELECT count(*) FROM tenant.cluster_outbox WHERE namespace_id IS NOT NULL`,
	).Scan(&count))
	require.Equal(t, 0, count, "no namespace outbox rows should be created when the cluster has no namespaces")
}
