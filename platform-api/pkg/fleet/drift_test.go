package fleet_test

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/platform-api/internal/testpg"
	fleetv1 "github.com/fundament-oss/fundament/platform-api/pkg/proto/gen/fleet/v1"
)

// The findings are cluster-worker's record; the API lists the open ones with
// the cluster and organization they concern.
func TestListDriftFindings(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	seed := env.Seed(t)
	client := staffClient(t, env)

	org := seed.Organization("drift-org", "Drift Org")
	missing := seed.Cluster(org, "missing")
	drifted := seed.Cluster(org, "drifted")
	stray := uuid.New()

	seed.DriftFinding(testpg.Finding{Kind: "cluster_without_shoot", ClusterID: missing, FirstDetectedAgo: time.Hour})
	seed.DriftFinding(testpg.Finding{
		Kind: "kubernetes_version_mismatch", ClusterID: drifted, ShootName: "drifted-shoot",
		Expected: "1.31.1", Observed: "1.32.0", FirstDetectedAgo: time.Minute,
	})
	seed.DriftFinding(testpg.Finding{Kind: "shoot_without_cluster", ClusterID: stray, ShootName: "stray-shoot", FirstDetectedAgo: time.Minute})
	seed.DriftFinding(testpg.Finding{Kind: "shoot_invalid_cluster_id", ShootName: "odd-shoot", Observed: "not-a-uuid", FirstDetectedAgo: time.Minute})
	seed.DriftFinding(testpg.Finding{Kind: "cluster_without_shoot", ClusterID: drifted, FirstDetectedAgo: 2 * time.Hour, Resolved: true})

	resp, err := client.ListDriftFindings(t.Context(), &fleetv1.ListDriftFindingsRequest{})
	require.NoError(t, err)
	findings := resp.GetFindings()
	require.Len(t, findings, 4, "a resolved finding is history, not drift")
	assert.Empty(t, resp.GetNextPageToken())

	// Grouped by kind, then oldest first.
	byKind := map[fleetv1.DriftKind]*fleetv1.DriftFinding{}
	for _, f := range findings {
		byKind[f.GetKind()] = f
	}

	without := byKind[fleetv1.DriftKind_DRIFT_KIND_CLUSTER_WITHOUT_SHOOT]
	require.NotNil(t, without)
	assert.Equal(t, missing.String(), without.GetClusterId())
	assert.Equal(t, "missing", without.GetClusterName())
	assert.Equal(t, "drift-org", without.GetOrganizationName())
	assert.WithinDuration(t, time.Now().Add(-time.Hour), without.GetFirstDetected().AsTime(), time.Minute)

	mismatch := byKind[fleetv1.DriftKind_DRIFT_KIND_KUBERNETES_VERSION_MISMATCH]
	require.NotNil(t, mismatch)
	assert.Equal(t, "The database has Kubernetes 1.31.1, the shoot runs 1.32.0.", mismatch.GetDetail())
	assert.Equal(t, "drifted-shoot", mismatch.GetShootName())

	unknown := byKind[fleetv1.DriftKind_DRIFT_KIND_SHOOT_WITHOUT_CLUSTER]
	require.NotNil(t, unknown)
	assert.Equal(t, stray.String(), unknown.GetClusterId())
	assert.False(t, unknown.HasOrganizationId(), "no cluster, so no organization")
	assert.False(t, unknown.HasClusterName())

	invalid := byKind[fleetv1.DriftKind_DRIFT_KIND_SHOOT_INVALID_CLUSTER_ID]
	require.NotNil(t, invalid)
	assert.False(t, invalid.HasClusterId(), "the label is no id")
	assert.Contains(t, invalid.GetDetail(), "not-a-uuid")

	summary, err := client.GetFleetSummary(t.Context(), &fleetv1.GetFleetSummaryRequest{})
	require.NoError(t, err)
	assert.Equal(t, int32(4), summary.GetSummary().GetDriftFindings())
}

func TestListDriftFindingsPages(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	seed := env.Seed(t)
	client := staffClient(t, env)

	for i := range 5 {
		seed.DriftFinding(testpg.Finding{Kind: "shoot_without_cluster", ClusterID: uuid.New(), ShootName: "stray", FirstDetectedAgo: time.Duration(i) * time.Minute})
	}

	var seen []string
	token := ""
	for range 4 {
		resp, err := client.ListDriftFindings(t.Context(), fleetv1.ListDriftFindingsRequest_builder{PageSize: 2, PageToken: token}.Build())
		require.NoError(t, err)
		assert.LessOrEqual(t, len(resp.GetFindings()), 2)
		for _, f := range resp.GetFindings() {
			seen = append(seen, f.GetId())
		}
		token = resp.GetNextPageToken()
		if token == "" {
			break
		}
	}
	assert.Len(t, seen, 5)
	assert.Empty(t, token, "three pages hold five findings")

	_, err := client.ListDriftFindings(t.Context(), fleetv1.ListDriftFindingsRequest_builder{PageToken: "not a token"}.Build())
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// Findings on the system organization's clusters are no tenant's drift.
func TestListDriftFindingsSkipsSystemOrganization(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	seed := env.Seed(t)
	client := staffClient(t, env)

	system := seed.Cluster(testpg.SystemOrganizationID, "plugins")
	seed.DriftFinding(testpg.Finding{Kind: "cluster_without_shoot", ClusterID: system})

	resp, err := client.ListDriftFindings(t.Context(), &fleetv1.ListDriftFindingsRequest{})
	require.NoError(t, err)
	assert.Empty(t, resp.GetFindings())
}
