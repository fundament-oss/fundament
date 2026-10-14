package fleet_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/platform-api/internal/testpg"
	fleetv1 "github.com/fundament-oss/fundament/platform-api/pkg/proto/gen/fleet/v1"
	"github.com/fundament-oss/fundament/platform-api/pkg/proto/gen/fleet/v1/fleetv1connect"
)

// findingsFor returns the findings of one kind for one cluster id.
func findingsFor(t *testing.T, client fleetv1connect.FleetServiceClient, kind fleetv1.DriftKind, clusterID uuid.UUID) []*fleetv1.DriftFinding {
	t.Helper()
	resp, err := client.ListDriftFindings(t.Context(), &fleetv1.ListDriftFindingsRequest{})
	require.NoError(t, err)
	var out []*fleetv1.DriftFinding
	for _, f := range resp.GetFindings() {
		if f.GetKind() == kind && f.GetClusterId() == clusterID.String() {
			out = append(out, f)
		}
	}
	return out
}

func TestDriftShootWithoutCluster(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	seed := env.Seed(t)
	client := staffClient(t, env)

	unknown := uuid.New()
	seed.GardenerShoot(unknown, "stray-shoot", "1.31.1", time.Minute)

	found := findingsFor(t, client, fleetv1.DriftKind_DRIFT_KIND_SHOOT_WITHOUT_CLUSTER, unknown)
	require.Len(t, found, 1)
	assert.Equal(t, "stray-shoot", found[0].GetShootName())
	assert.False(t, found[0].HasOrganizationId(), "no cluster, so no organization")
	assert.True(t, found[0].HasSince())
}

func TestDriftShootOfDeletedCluster(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	seed := env.Seed(t)
	client := staffClient(t, env)

	org := seed.Organization("drift-deleted", "Drift Deleted")
	cluster := seed.Cluster(org, "torn-down")
	seed.GardenerShoot(cluster, "torn-down-shoot", "1.31.1", time.Minute)
	seed.SoftDelete("clusters", cluster)

	found := findingsFor(t, client, fleetv1.DriftKind_DRIFT_KIND_SHOOT_OF_DELETED_CLUSTER, cluster)
	require.Len(t, found, 1)
	assert.Equal(t, "torn-down", found[0].GetClusterName())
	assert.Equal(t, "drift-deleted", found[0].GetOrganizationName())
	assert.True(t, found[0].HasSince(), "since the cluster was deleted")

	assert.Empty(t, findingsFor(t, client, fleetv1.DriftKind_DRIFT_KIND_SHOOT_WITHOUT_CLUSTER, cluster),
		"a deleted cluster still exists, so the shoot is not one without a cluster")
}

func TestDriftClusterWithoutShoot(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	seed := env.Seed(t)
	client := staffClient(t, env)

	org := seed.Organization("drift-missing", "Drift Missing")
	missing := seed.Cluster(org, "missing")
	seed.Ready(missing, time.Hour)

	// Changed after the last refresh: the inventory may simply not have seen
	// its shoot yet.
	recent := seed.Cluster(org, "recent")
	seed.Ready(recent, 0)

	present := seed.Cluster(org, "present")
	seed.Ready(present, time.Hour)
	seed.GardenerShoot(present, "present-shoot", "1.31.1", time.Minute)

	found := findingsFor(t, client, fleetv1.DriftKind_DRIFT_KIND_CLUSTER_WITHOUT_SHOOT, missing)
	require.Len(t, found, 1)
	assert.Equal(t, "missing", found[0].GetClusterName())

	assert.Empty(t, findingsFor(t, client, fleetv1.DriftKind_DRIFT_KIND_CLUSTER_WITHOUT_SHOOT, recent))
	assert.Empty(t, findingsFor(t, client, fleetv1.DriftKind_DRIFT_KIND_CLUSTER_WITHOUT_SHOOT, present))
}

// With the inventory last refreshed longer ago than stale_after, a stopped
// worker would make every cluster look missing; nothing is reported.
func TestDriftClusterWithoutShoot_StaleInventory(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	seed := env.Seed(t)
	client := staffClient(t, env)

	org := seed.Organization("drift-stale-inv", "Drift Stale Inventory")
	missing := seed.Cluster(org, "missing")
	seed.Ready(missing, 3*time.Hour)
	other := seed.Cluster(org, "other")
	seed.Ready(other, 3*time.Hour)
	seed.GardenerShoot(other, "other-shoot", "1.31.1", 2*time.Hour)

	assert.Empty(t, findingsFor(t, client, fleetv1.DriftKind_DRIFT_KIND_CLUSTER_WITHOUT_SHOOT, missing))
}

func TestDriftSnapshotStale(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	seed := env.Seed(t)
	client := staffClient(t, env)

	org := seed.Organization("drift-snapshot", "Drift Snapshot")

	never := seed.Cluster(org, "never")
	seed.Ready(never, 2*time.Hour)

	old := seed.Cluster(org, "old")
	seed.Ready(old, 3*time.Hour)
	seed.ClusterSnapshot(old, 2*time.Hour, 1, 1, 1, 110)
	_, err := env.Super.Exec(t.Context(),
		`UPDATE tenant.cluster_snapshots SET last_error = 'dial tcp: i/o timeout', last_error_at = now() WHERE cluster_id = $1`, old)
	require.NoError(t, err)

	justReady := seed.Cluster(org, "just-ready")
	seed.Ready(justReady, time.Minute)

	fresh := seed.Cluster(org, "fresh")
	seed.Ready(fresh, 3*time.Hour)
	seed.ClusterSnapshot(fresh, time.Minute, 1, 1, 1, 110)

	neverFound := findingsFor(t, client, fleetv1.DriftKind_DRIFT_KIND_SNAPSHOT_STALE, never)
	require.Len(t, neverFound, 1)
	assert.Equal(t, "Never collected.", neverFound[0].GetDetail())

	oldFound := findingsFor(t, client, fleetv1.DriftKind_DRIFT_KIND_SNAPSHOT_STALE, old)
	require.Len(t, oldFound, 1)
	assert.Contains(t, oldFound[0].GetDetail(), "Last collected")
	assert.Contains(t, oldFound[0].GetDetail(), "i/o timeout")

	assert.Empty(t, findingsFor(t, client, fleetv1.DriftKind_DRIFT_KIND_SNAPSHOT_STALE, justReady),
		"a cluster ready for less than stale_after gets time for its first collection")
	assert.Empty(t, findingsFor(t, client, fleetv1.DriftKind_DRIFT_KIND_SNAPSHOT_STALE, fresh))

	summary, err := client.GetFleetSummary(t.Context(), &fleetv1.GetFleetSummaryRequest{})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, summary.GetSummary().GetStaleSnapshots(), int32(2))
	assert.GreaterOrEqual(t, summary.GetSummary().GetDriftFindings(), summary.GetSummary().GetStaleSnapshots())
}

func TestDriftKubernetesVersionMismatch(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	seed := env.Seed(t)
	client := staffClient(t, env)

	org := seed.Organization("drift-version", "Drift Version")

	drifted := seed.Cluster(org, "drifted")
	seed.Ready(drifted, time.Hour)
	seed.GardenerShoot(drifted, "drifted-shoot", "1.32.0", time.Minute)

	updating := seed.Cluster(org, "updating")
	seed.Ready(updating, time.Hour)
	seed.Status(updating, "ready", "healthy", true)
	seed.GardenerShoot(updating, "updating-shoot", "1.32.0", time.Minute)

	// The inventory saw the shoot before the cluster's last status change, so
	// its version may predate an update fundament just applied.
	unseen := seed.Cluster(org, "unseen")
	seed.Ready(unseen, time.Minute)
	seed.GardenerShoot(unseen, "unseen-shoot", "1.32.0", time.Hour)

	matching := seed.Cluster(org, "matching")
	seed.Ready(matching, time.Hour)
	seed.GardenerShoot(matching, "matching-shoot", "1.31.1", time.Minute)

	found := findingsFor(t, client, fleetv1.DriftKind_DRIFT_KIND_KUBERNETES_VERSION_MISMATCH, drifted)
	require.Len(t, found, 1)
	assert.Equal(t, "The database has Kubernetes 1.31.1, the shoot runs 1.32.0.", found[0].GetDetail())
	assert.Equal(t, "drifted-shoot", found[0].GetShootName())

	assert.Empty(t, findingsFor(t, client, fleetv1.DriftKind_DRIFT_KIND_KUBERNETES_VERSION_MISMATCH, updating))
	assert.Empty(t, findingsFor(t, client, fleetv1.DriftKind_DRIFT_KIND_KUBERNETES_VERSION_MISMATCH, unseen))
	assert.Empty(t, findingsFor(t, client, fleetv1.DriftKind_DRIFT_KIND_KUBERNETES_VERSION_MISMATCH, matching))
}
