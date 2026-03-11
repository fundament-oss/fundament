package cluster_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler/cluster"
)

// Well-known test data IDs from db/testdata/001_0101-content.sql.
var acmeCorpOrgID = uuid.MustParse("019b4000-0000-7000-8000-000000000001")

type testDB struct {
	adminPool  *pgxpool.Pool // postgres superuser — for setup + assertions
	workerPool *pgxpool.Pool // fun_cluster_worker — for Handler (matches prod)
}

func createTestDB(t *testing.T) *testDB {
	t.Helper()

	name := testNameToDbName(t.Name())

	adminURL := fmt.Sprintf("postgres://postgres:postgres@localhost:%d/postgres?sslmode=disable", testDBPort)
	adminPool, err := pgxpool.New(context.Background(), adminURL)
	require.NoError(t, err)

	_, err = adminPool.Exec(context.Background(), fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, name))
	require.NoError(t, err)

	_, err = adminPool.Exec(context.Background(), fmt.Sprintf(`CREATE DATABASE %q TEMPLATE fundament`, name))
	require.NoError(t, err)

	adminPool.Close()

	// Reconnect to the new test database.
	testAdminPool, err := pgxpool.New(context.Background(),
		fmt.Sprintf("postgres://postgres:postgres@localhost:%d/%s?sslmode=disable", testDBPort, name))
	require.NoError(t, err)

	workerPool, err := pgxpool.New(context.Background(),
		fmt.Sprintf("postgres://fun_cluster_worker:fun_cluster_worker@localhost:%d/%s?sslmode=disable", testDBPort, name))
	require.NoError(t, err)

	t.Cleanup(func() {
		workerPool.Close()
		testAdminPool.Close()
	})

	return &testDB{
		adminPool:  testAdminPool,
		workerPool: workerPool,
	}
}

func testNameToDbName(testName string) string {
	name := strings.ToLower(testName)
	name = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}

func newTestHandler(t *testing.T, db *testDB, mock *gardener.MockClient) *cluster.Handler {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	cfg := cluster.Config{
		StatusBatchSize: 50,
		MaxRetries:      10,
	}
	return cluster.New(db.workerPool, mock, logger, cfg)
}

func newMock(t *testing.T) *gardener.MockClient {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	return gardener.NewMockInstant(logger)
}

// insertCluster inserts a cluster into the test DB. The trigger auto-creates an
// outbox row with status=pending (which propagates to clusters.outbox_status).
func insertCluster(t *testing.T, db *testDB, orgID uuid.UUID, name string) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	err := db.adminPool.QueryRow(t.Context(),
		`INSERT INTO tenant.clusters (organization_id, name, region, kubernetes_version)
		 VALUES ($1, $2, 'eu-west-1', '1.31.1')
		 RETURNING id`,
		orgID, name,
	).Scan(&id)
	require.NoError(t, err)

	return id
}

// insertDeletedCluster inserts a soft-deleted cluster.
func insertDeletedCluster(t *testing.T, db *testDB, orgID uuid.UUID, name string) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	err := db.adminPool.QueryRow(t.Context(),
		`INSERT INTO tenant.clusters (organization_id, name, region, kubernetes_version, deleted)
		 VALUES ($1, $2, 'eu-west-1', '1.31.1', now())
		 RETURNING id`,
		orgID, name,
	).Scan(&id)
	require.NoError(t, err)

	return id
}

// insertNodePool inserts a node pool for a cluster.
func insertNodePool(t *testing.T, db *testDB, clusterID uuid.UUID, name, machineType string, scaleMin, scaleMax int32) {
	t.Helper()

	_, err := db.adminPool.Exec(t.Context(),
		`INSERT INTO tenant.node_pools (cluster_id, name, machine_type, autoscale_min, autoscale_max)
		 VALUES ($1, $2, $3, $4, $5)`,
		clusterID, name, machineType, scaleMin, scaleMax,
	)
	require.NoError(t, err)
}

// markOutboxCompleted sets the latest outbox row for a cluster to completed.
// The trigger propagates this to clusters.outbox_status.
func markOutboxCompleted(t *testing.T, db *testDB, clusterID uuid.UUID) {
	t.Helper()

	result, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.cluster_outbox
		 SET status = 'completed', processed = now()
		 WHERE id = (
		     SELECT id FROM tenant.cluster_outbox
		     WHERE cluster_id = $1
		     ORDER BY id DESC
		     LIMIT 1
		 )`,
		clusterID,
	)
	require.NoError(t, err)
	require.EqualValues(t, 1, result.RowsAffected(), "expected exactly one outbox row to be marked completed")
}

// setShootStatus directly sets shoot_status with updated = now() - 1 min to avoid
// the 30-second throttle in status queries.
func setShootStatus(t *testing.T, db *testDB, clusterID uuid.UUID, status string) {
	t.Helper()

	result, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.clusters
		 SET shoot_status = $1, shoot_status_updated = now() - interval '1 minute'
		 WHERE id = $2`,
		status, clusterID,
	)
	require.NoError(t, err)
	require.EqualValues(t, 1, result.RowsAffected(), "expected exactly one cluster row to be updated")
}

// assertEventExists asserts that at least one cluster event of the given type exists.
func assertEventExists(t *testing.T, db *testDB, clusterID uuid.UUID, eventType string) {
	t.Helper()

	var count int
	err := db.adminPool.QueryRow(t.Context(),
		`SELECT count(*) FROM tenant.cluster_events
		 WHERE cluster_id = $1 AND event_type = $2`,
		clusterID, eventType,
	).Scan(&count)
	require.NoError(t, err)
	require.Greaterf(t, count, 0, "expected at least one %q event for cluster %s", eventType, clusterID)
}

// assertNoEvent asserts that no cluster event of the given type exists.
func assertNoEvent(t *testing.T, db *testDB, clusterID uuid.UUID, eventType string) {
	t.Helper()

	var count int
	err := db.adminPool.QueryRow(t.Context(),
		`SELECT count(*) FROM tenant.cluster_events
		 WHERE cluster_id = $1 AND event_type = $2`,
		clusterID, eventType,
	).Scan(&count)
	require.NoError(t, err)
	require.Equalf(t, 0, count, "expected no %q event for cluster %s", eventType, clusterID)
}

// assertOutboxReconcileExists asserts a reconcile outbox row exists for the cluster.
func assertOutboxReconcileExists(t *testing.T, db *testDB, clusterID uuid.UUID) {
	t.Helper()

	var count int
	err := db.adminPool.QueryRow(t.Context(),
		`SELECT count(*) FROM tenant.cluster_outbox
		 WHERE cluster_id = $1 AND event = 'reconcile'`,
		clusterID,
	).Scan(&count)
	require.NoError(t, err)
	require.Greaterf(t, count, 0, "expected a reconcile outbox row for cluster %s", clusterID)
}

// assertNoOutboxReconcile asserts no reconcile outbox row exists for the cluster.
func assertNoOutboxReconcile(t *testing.T, db *testDB, clusterID uuid.UUID) {
	t.Helper()

	var count int
	err := db.adminPool.QueryRow(t.Context(),
		`SELECT count(*) FROM tenant.cluster_outbox
		 WHERE cluster_id = $1 AND event = 'reconcile'`,
		clusterID,
	).Scan(&count)
	require.NoError(t, err)
	require.Equalf(t, 0, count, "expected no reconcile outbox row for cluster %s", clusterID)
}

// getClusterShootStatus returns the current shoot_status of a cluster (nil if NULL).
func getClusterShootStatus(t *testing.T, db *testDB, clusterID uuid.UUID) *string {
	t.Helper()

	var status *string
	err := db.adminPool.QueryRow(t.Context(),
		`SELECT shoot_status FROM tenant.clusters WHERE id = $1`,
		clusterID,
	).Scan(&status)
	require.NoError(t, err)
	return status
}
