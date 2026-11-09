package cluster_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler/cluster"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/inventory"
	"github.com/fundament-oss/fundament/common/kubename"
)

func newInventoryWorker(t *testing.T, db *testDB, reader shoot.InventoryReader) *inventory.Worker {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	return inventory.New(db.workerPool, reader, logger, inventory.Config{
		Interval: time.Hour,
		Workers:  1,
		Timeout:  5 * time.Second,
	})
}

type clusterSnapshotRow struct {
	observed                                *time.Time
	nodes, nodesReady, pods, podAllocatable int32
	cpuAllocatableM, memoryAllocatableMi    int64
	lastError                               *string
	lastErrorAt                             *time.Time
}

func getClusterSnapshot(t *testing.T, db *testDB, clusterID uuid.UUID) clusterSnapshotRow {
	t.Helper()
	var r clusterSnapshotRow
	err := db.adminPool.QueryRow(t.Context(),
		`SELECT observed, nodes, nodes_ready, pods, pod_allocatable, cpu_allocatable_m,
		        memory_allocatable_mi, last_error, last_error_at
		 FROM tenant.cluster_snapshots WHERE cluster_id = $1`, clusterID,
	).Scan(&r.observed, &r.nodes, &r.nodesReady, &r.pods, &r.podAllocatable,
		&r.cpuAllocatableM, &r.memoryAllocatableMi, &r.lastError, &r.lastErrorAt)
	require.NoError(t, err)
	return r
}

func countRows(t *testing.T, db *testDB, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, db.adminPool.QueryRow(t.Context(), query, args...).Scan(&n))
	return n
}

// fakeInventory is the shoot a test collects from: it reports inv, or err.
type fakeInventory struct {
	mu  sync.Mutex
	inv shoot.Inventory
	err error
}

func (f *fakeInventory) ReadInventory(context.Context, uuid.UUID) (*shoot.Inventory, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	inv := f.inv
	return &inv, nil
}

func (f *fakeInventory) set(inv shoot.Inventory, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inv, f.err = inv, err
}

// inventoryOf is a shoot running nodes nodes in the workers pool, with pods
// pods in the namespace with namespaceID.
func inventoryOf(nodes int32, namespaceID uuid.UUID, pods int32) shoot.Inventory {
	return shoot.Inventory{
		Nodes:               nodes,
		NodesReady:          nodes,
		Pods:                nodes*8 + pods,
		PodAllocatable:      nodes * 110,
		CPUAllocatableMilli: int64(nodes) * 4000,
		MemoryAllocatableMi: int64(nodes) * 16384,
		Pools: map[string]shoot.PoolInventory{
			kubename.GenerateWorkerName("workers"): {Nodes: nodes, NodesReady: nodes},
		},
		NamespacePods: map[uuid.UUID]int32{namespaceID: pods},
	}
}

// inventoryFixture is a ready cluster with two node pools and two namespaces,
// one of which the shoot does not carry yet.
type inventoryFixture struct {
	clusterID, poolID, idlePoolID, namespaceID, missingNamespaceID uuid.UUID
	reader                                                         *fakeInventory
}

func newInventoryFixture(t *testing.T, db *testDB) inventoryFixture {
	t.Helper()

	clusterID := insertCluster(t, db, acmeCorpOrgID, "inventory")
	setShootStatus(t, db, clusterID, "ready")
	poolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-2", 2, 5)
	idlePoolID := insertNodePoolReturningID(t, db, clusterID, "idle", "n1-standard-2", 0, 3)
	projectID := insertProject(t, db, clusterID, "inventory-project")
	namespaceID := insertNamespace(t, db, projectID, "web")
	missingNamespaceID := insertNamespace(t, db, projectID, "not-yet-created")

	reader := &fakeInventory{inv: inventoryOf(2, namespaceID, 5)}

	return inventoryFixture{clusterID, poolID, idlePoolID, namespaceID, missingNamespaceID, reader}
}

func TestInventoryCollect_WritesSnapshots(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	f := newInventoryFixture(t, db)
	w := newInventoryWorker(t, db, f.reader)

	require.NoError(t, w.Collect(t.Context(), f.clusterID))

	expected := f.reader.inv

	snap := getClusterSnapshot(t, db, f.clusterID)
	require.NotNil(t, snap.observed)
	assert.Equal(t, int32(2), snap.nodes)
	assert.Equal(t, int32(2), snap.nodesReady)
	assert.Equal(t, expected.Pods, snap.pods)
	assert.Equal(t, expected.PodAllocatable, snap.podAllocatable)
	assert.Equal(t, expected.CPUAllocatableMilli, snap.cpuAllocatableM)
	assert.Equal(t, expected.MemoryAllocatableMi, snap.memoryAllocatableMi)
	assert.Nil(t, snap.lastError)

	var nodes, ready int32
	require.NoError(t, db.adminPool.QueryRow(t.Context(),
		`SELECT nodes, nodes_ready FROM tenant.node_pool_snapshots WHERE node_pool_id = $1`, f.poolID,
	).Scan(&nodes, &ready))
	assert.Equal(t, int32(2), nodes, "pools match the shoot's workers by their Gardener worker name")
	assert.Equal(t, int32(2), ready)

	require.NoError(t, db.adminPool.QueryRow(t.Context(),
		`SELECT nodes FROM tenant.node_pool_snapshots WHERE node_pool_id = $1`, f.idlePoolID,
	).Scan(&nodes))
	assert.Equal(t, int32(0), nodes, "a pool Gardener runs no nodes for is recorded as zero")

	var pods int32
	require.NoError(t, db.adminPool.QueryRow(t.Context(),
		`SELECT pods FROM tenant.namespace_snapshots WHERE namespace_id = $1`, f.namespaceID,
	).Scan(&pods))
	assert.Equal(t, expected.NamespacePods[f.namespaceID], pods)

	assert.Zero(t, countRows(t, db,
		`SELECT count(*) FROM tenant.namespace_snapshots WHERE namespace_id = $1`, f.missingNamespaceID),
		"a namespace missing on the shoot gets no snapshot rather than zero pods")
}

func TestInventoryCollect_Idempotent(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	f := newInventoryFixture(t, db)
	w := newInventoryWorker(t, db, f.reader)

	require.NoError(t, w.Collect(t.Context(), f.clusterID))
	first := getClusterSnapshot(t, db, f.clusterID)

	// The pool scales up between collections.
	f.reader.set(inventoryOf(4, f.namespaceID, 5), nil)
	require.NoError(t, w.Collect(t.Context(), f.clusterID))
	second := getClusterSnapshot(t, db, f.clusterID)

	assert.Equal(t, 1, countRows(t, db, `SELECT count(*) FROM tenant.cluster_snapshots WHERE cluster_id = $1`, f.clusterID))
	assert.Equal(t, 1, countRows(t, db, `SELECT count(*) FROM tenant.node_pool_snapshots WHERE node_pool_id = $1`, f.poolID))
	assert.Equal(t, 1, countRows(t, db, `SELECT count(*) FROM tenant.namespace_snapshots WHERE namespace_id = $1`, f.namespaceID))
	assert.Equal(t, int32(4), second.nodes)
	assert.False(t, second.observed.Before(*first.observed))
}

func TestInventoryCollect_ErrorKeepsLastObservation(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	f := newInventoryFixture(t, db)
	w := newInventoryWorker(t, db, f.reader)

	require.NoError(t, w.Collect(t.Context(), f.clusterID))
	good := getClusterSnapshot(t, db, f.clusterID)

	f.reader.set(f.reader.inv, errors.New("dial tcp: connection refused"))
	require.ErrorContains(t, w.Collect(t.Context(), f.clusterID), "connection refused")

	failed := getClusterSnapshot(t, db, f.clusterID)
	require.NotNil(t, failed.lastError)
	assert.Contains(t, *failed.lastError, "connection refused")
	assert.NotNil(t, failed.lastErrorAt)
	assert.Equal(t, good.observed, failed.observed, "the last good observation stays")
	assert.Equal(t, good.nodes, failed.nodes)

	// The next good collection clears the error.
	f.reader.set(f.reader.inv, nil)
	require.NoError(t, w.Collect(t.Context(), f.clusterID))
	recovered := getClusterSnapshot(t, db, f.clusterID)
	assert.Nil(t, recovered.lastError)
	assert.Nil(t, recovered.lastErrorAt)
}

func TestInventoryCollect_FirstCollectionFails(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	f := newInventoryFixture(t, db)
	f.reader.set(f.reader.inv, errors.New("forbidden"))
	w := newInventoryWorker(t, db, f.reader)

	require.Error(t, w.Collect(t.Context(), f.clusterID))

	snap := getClusterSnapshot(t, db, f.clusterID)
	assert.Nil(t, snap.observed, "never observed")
	require.NotNil(t, snap.lastError)
	assert.Equal(t, "read inventory: forbidden", *snap.lastError)
}

// A collection that reads but cannot store records why, like a failed read.
func TestInventoryCollect_StoreFailureIsRecorded(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	f := newInventoryFixture(t, db)
	w := newInventoryWorker(t, db, f.reader)

	require.NoError(t, w.Collect(t.Context(), f.clusterID))
	good := getClusterSnapshot(t, db, f.clusterID)

	// More ready nodes than nodes breaks cluster_snapshots_ck_counts.
	broken := inventoryOf(2, f.namespaceID, 5)
	broken.NodesReady = 3
	f.reader.set(broken, nil)
	require.ErrorContains(t, w.Collect(t.Context(), f.clusterID), "store inventory")

	failed := getClusterSnapshot(t, db, f.clusterID)
	require.NotNil(t, failed.lastError)
	assert.Contains(t, *failed.lastError, "store inventory")
	assert.Equal(t, good.observed, failed.observed, "the last good observation stays")
	assert.Equal(t, good.nodes, failed.nodes)
}

// Each pass records the interval the worker runs at, which the fleet view
// judges snapshots by.
func TestInventoryEnqueueAll_RecordsHeartbeat(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	w := newInventoryWorker(t, db, &fakeInventory{})

	w.EnqueueAll(t.Context())

	var interval int32
	var lastPass time.Time
	require.NoError(t, db.adminPool.QueryRow(t.Context(),
		`SELECT interval_seconds, last_pass FROM tenant.worker_heartbeats WHERE worker = 'inventory'`,
	).Scan(&interval, &lastPass))
	assert.Equal(t, int32(3600), interval)
	assert.WithinDuration(t, time.Now(), lastPass, time.Minute)

	w.EnqueueAll(t.Context())
	assert.Equal(t, 1, countRows(t, db, `SELECT count(*) FROM tenant.worker_heartbeats WHERE worker = 'inventory'`))
}

// In mock mode the inventory runs apart from the mock Gardener, so it reads
// the database: every live pool at its minimum, every live namespace.
func TestMockReader(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	f := newInventoryFixture(t, db)
	r := inventory.NewMockReader(db.workerPool)

	first, err := r.ReadInventory(t.Context(), f.clusterID)
	require.NoError(t, err)
	second, err := r.ReadInventory(t.Context(), f.clusterID)
	require.NoError(t, err)

	assert.Equal(t, first, second, "the mock reports the same numbers every collection")
	assert.Equal(t, int32(2), first.Nodes)
	assert.Equal(t, int32(2), first.NodesReady)
	assert.Equal(t, shoot.PoolInventory{Nodes: 2, NodesReady: 2}, first.Pools[kubename.GenerateWorkerName("workers")])
	assert.Equal(t, shoot.PoolInventory{}, first.Pools[kubename.GenerateWorkerName("idle")])
	assert.Contains(t, first.NamespacePods, f.namespaceID)
	assert.Contains(t, first.NamespacePods, f.missingNamespaceID)
	assert.Equal(t, 2*8+first.NamespacePods[f.namespaceID]+first.NamespacePods[f.missingNamespaceID], first.Pods)
}

func isInsufficientPrivilege(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.InsufficientPrivilege
}

// Snapshots and the Gardener inventory are kept, never removed: the worker's
// role has no DELETE grant, so the database enforces it.
func TestInventoryTables_WorkerCannotDelete(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	for _, table := range []string{
		"tenant.cluster_snapshots",
		"tenant.node_pool_snapshots",
		"tenant.namespace_snapshots",
		"tenant.gardener_shoots",
		"tenant.drift_findings",
		"tenant.worker_heartbeats",
	} {
		_, err := db.workerPool.Exec(t.Context(), "DELETE FROM "+table)
		require.Error(t, err, table)
		assert.True(t, isInsufficientPrivilege(err), "%s: expected insufficient_privilege, got %v", table, err)
	}
}

func applyMockShoot(t *testing.T, mock *gardener.MockClient, clusterID uuid.UUID, name, version string) {
	t.Helper()
	_, err := mock.ApplyShoot(t.Context(), &gardener.ClusterToSync{
		ID:                clusterID,
		OrganizationID:    acmeCorpOrgID,
		OrganizationName:  "acme-corp",
		Name:              name,
		ShootName:         kubename.GenerateShootName(name, clusterID),
		Namespace:         "garden-acme",
		Region:            "eu-west-1",
		KubernetesVersion: version,
	})
	require.NoError(t, err)
}

type gardenerShootRow struct {
	clusterID           uuid.UUID
	kubernetesVersion   string
	region              string
	firstSeen, lastSeen time.Time
	deleted             *time.Time
}

func gardenerShoots(t *testing.T, db *testDB) map[uuid.UUID]gardenerShootRow {
	t.Helper()
	rows, err := db.adminPool.Query(t.Context(),
		`SELECT cluster_id, kubernetes_version, region, first_seen, last_seen, deleted FROM tenant.gardener_shoots`)
	require.NoError(t, err)
	defer rows.Close()
	out := map[uuid.UUID]gardenerShootRow{}
	for rows.Next() {
		var r gardenerShootRow
		require.NoError(t, rows.Scan(&r.clusterID, &r.kubernetesVersion, &r.region, &r.firstSeen, &r.lastSeen, &r.deleted))
		out[r.clusterID] = r
	}
	require.NoError(t, rows.Err())
	return out
}

func TestReconcileRecordsGardenerInventory(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	keptID := insertCluster(t, db, acmeCorpOrgID, "kept")
	markOutboxCompleted(t, db, keptID)
	goneID := insertCluster(t, db, acmeCorpOrgID, "gone")
	markOutboxCompleted(t, db, goneID)
	applyMockShoot(t, mock, keptID, "kept", "1.31.1")
	applyMockShoot(t, mock, goneID, "gone", "1.31.1")

	require.NoError(t, h.Reconcile(t.Context()))

	first := gardenerShoots(t, db)
	require.Len(t, first, 2)
	assert.Equal(t, "1.31.1", first[keptID].kubernetesVersion)
	assert.Equal(t, "eu-west-1", first[keptID].region)
	assert.Nil(t, first[keptID].deleted)

	// One shoot disappears from Gardener and the other is updated.
	_, err := mock.DeleteShootByClusterID(t.Context(), goneID)
	require.NoError(t, err)
	applyMockShoot(t, mock, keptID, "kept", "1.32.0")

	require.NoError(t, h.Reconcile(t.Context()))

	second := gardenerShoots(t, db)
	require.Len(t, second, 2, "a shoot that is gone is soft-deleted, not removed")
	assert.Equal(t, "1.32.0", second[keptID].kubernetesVersion)
	assert.Equal(t, first[keptID].firstSeen, second[keptID].firstSeen, "first_seen stays")
	assert.True(t, second[keptID].lastSeen.After(first[keptID].lastSeen), "last_seen moves")
	assert.Nil(t, second[keptID].deleted)
	assert.NotNil(t, second[goneID].deleted)
}

func TestReconcileFailedListMarksNothingGone(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "listed")
	markOutboxCompleted(t, db, clusterID)
	applyMockShoot(t, mock, clusterID, "listed", "1.31.1")
	require.NoError(t, h.Reconcile(t.Context()))

	mock.ListError = errors.New("garden unreachable")
	require.Error(t, h.Reconcile(t.Context()))

	shoots := gardenerShoots(t, db)
	require.Contains(t, shoots, clusterID)
	assert.Nil(t, shoots[clusterID].deleted, "a failed list says nothing about which shoots are gone")
}

func TestReconcileEmptyListMarksEverythingGone(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "last-one")
	markOutboxCompleted(t, db, clusterID)
	applyMockShoot(t, mock, clusterID, "last-one", "1.31.1")
	require.NoError(t, h.Reconcile(t.Context()))

	_, err := mock.DeleteShootByClusterID(t.Context(), clusterID)
	require.NoError(t, err)
	require.NoError(t, h.Reconcile(t.Context()))

	assert.NotNil(t, gardenerShoots(t, db)[clusterID].deleted)
}

type driftFindingRow struct {
	kind                        string
	clusterID                   *uuid.UUID
	expected, observed          *string
	firstDetected, lastDetected time.Time
	resolved                    *time.Time
}

// driftFindings returns every finding of a cluster, open and resolved, oldest
// first.
func driftFindings(t *testing.T, db *testDB, clusterID uuid.UUID) []driftFindingRow {
	t.Helper()
	rows, err := db.adminPool.Query(t.Context(),
		`SELECT kind, cluster_id, expected, observed, first_detected, last_detected, resolved
		 FROM tenant.drift_findings WHERE cluster_id = $1 ORDER BY first_detected`, clusterID)
	require.NoError(t, err)
	defer rows.Close()
	var out []driftFindingRow
	for rows.Next() {
		var r driftFindingRow
		require.NoError(t, rows.Scan(&r.kind, &r.clusterID, &r.expected, &r.observed, &r.firstDetected, &r.lastDetected, &r.resolved))
		out = append(out, r)
	}
	require.NoError(t, rows.Err())
	return out
}

// A finding stays open while every pass sees it, keeping when it was first
// detected, and is resolved by the first pass that no longer does.
func TestReconcileRecordsClusterWithoutShoot(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "shootless")
	markOutboxCompleted(t, db, clusterID)

	require.NoError(t, h.Reconcile(t.Context()))
	require.NoError(t, h.Reconcile(t.Context()))

	findings := driftFindings(t, db, clusterID)
	require.Len(t, findings, 1, "one open finding however many passes see it")
	assert.Equal(t, "cluster_without_shoot", findings[0].kind)
	assert.True(t, findings[0].lastDetected.After(findings[0].firstDetected))
	assert.Nil(t, findings[0].resolved)

	applyMockShoot(t, mock, clusterID, "shootless", "1.31.1")
	require.NoError(t, h.Reconcile(t.Context()))

	findings = driftFindings(t, db, clusterID)
	require.Len(t, findings, 1, "resolved, not deleted")
	assert.NotNil(t, findings[0].resolved)
}

// The blind spot of computing drift from the shoots recorded: with none left,
// freshness was unknown. Reconcile reports every cluster that lost its shoot.
func TestReconcileRecordsClusterWithoutShoot_WhenGardenerListsNone(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "emptied")
	markOutboxCompleted(t, db, clusterID)
	applyMockShoot(t, mock, clusterID, "emptied", "1.31.1")
	require.NoError(t, h.Reconcile(t.Context()))
	assert.Empty(t, driftFindings(t, db, clusterID))

	_, err := mock.DeleteShootByClusterID(t.Context(), clusterID)
	require.NoError(t, err)
	require.NoError(t, h.Reconcile(t.Context()))

	findings := driftFindings(t, db, clusterID)
	require.Len(t, findings, 1)
	assert.Equal(t, "cluster_without_shoot", findings[0].kind)
}

func TestReconcileRecordsShootFindings(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandlerWithConfig(t, db, mock, cluster.Config{MaxReconcileDeletions: 0})

	unknownID := uuid.New()
	applyMockShoot(t, mock, unknownID, "stray", "1.31.1")
	deletedID := insertDeletedCluster(t, db, acmeCorpOrgID, "torn-down")
	applyMockShoot(t, mock, deletedID, "torn-down", "1.31.1")

	require.NoError(t, h.Reconcile(t.Context()))

	unknown := driftFindings(t, db, unknownID)
	require.Len(t, unknown, 1)
	assert.Equal(t, "shoot_without_cluster", unknown[0].kind)
	deleted := driftFindings(t, db, deletedID)
	require.Len(t, deleted, 1)
	assert.Equal(t, "shoot_of_deleted_cluster", deleted[0].kind)
}

// Gardener's maintenance moves patch versions on its own; only another minor
// is drift.
func TestReconcileRecordsKubernetesVersionMismatch(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandler(t, db, mock)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "versioned") // 1.31.1
	markOutboxCompleted(t, db, clusterID)
	setShootStatus(t, db, clusterID, "ready")

	applyMockShoot(t, mock, clusterID, "versioned", "1.31.4")
	require.NoError(t, h.Reconcile(t.Context()))
	assert.Empty(t, driftFindings(t, db, clusterID), "a patch Gardener applied is no drift")

	applyMockShoot(t, mock, clusterID, "versioned", "1.32.0")
	require.NoError(t, h.Reconcile(t.Context()))

	findings := driftFindings(t, db, clusterID)
	require.Len(t, findings, 1)
	assert.Equal(t, "kubernetes_version_mismatch", findings[0].kind)
	require.NotNil(t, findings[0].expected)
	require.NotNil(t, findings[0].observed)
	assert.Equal(t, "1.31.1", *findings[0].expected)
	assert.Equal(t, "1.32.0", *findings[0].observed)
}

func TestReconcileRecordsHeartbeat(t *testing.T) {
	t.Parallel()

	db := createTestDB(t)
	mock := newMock(t)
	h := newTestHandlerWithConfig(t, db, mock, cluster.Config{ReconcileInterval: 2 * time.Minute})

	require.NoError(t, h.Reconcile(t.Context()))

	var interval int32
	require.NoError(t, db.adminPool.QueryRow(t.Context(),
		`SELECT interval_seconds FROM tenant.worker_heartbeats WHERE worker = 'reconcile'`,
	).Scan(&interval))
	assert.Equal(t, int32(120), interval)

	// A failed list is no completed pass.
	var before time.Time
	require.NoError(t, db.adminPool.QueryRow(t.Context(),
		`SELECT last_pass FROM tenant.worker_heartbeats WHERE worker = 'reconcile'`).Scan(&before))
	mock.ListError = errors.New("garden unreachable")
	require.Error(t, h.Reconcile(t.Context()))
	var after time.Time
	require.NoError(t, db.adminPool.QueryRow(t.Context(),
		`SELECT last_pass FROM tenant.worker_heartbeats WHERE worker = 'reconcile'`).Scan(&after))
	assert.Equal(t, before, after)
}
