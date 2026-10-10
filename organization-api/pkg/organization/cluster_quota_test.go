package organization_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

// quotaEnv is an organization at the database's default cluster quota (one),
// with a client that acts as one of its admins.
type quotaEnv struct {
	env    *testEnv
	orgID  uuid.UUID
	client organizationv1connect.ClusterServiceClient
	ctx    context.Context
}

func newQuotaEnv(t *testing.T) *quotaEnv {
	t.Helper()

	orgID := uuid.New()
	userID := uuid.New()
	env := newTestAPI(t,
		WithOrganization(orgID, "quota-org"),
		WithUser(&UserArgs{ID: userID, Name: "quota-admin", OrgIDs: []uuid.UUID{orgID}}),
	)

	// The harness gives organizations room; this is about the real default.
	_, err := env.adminPool.Exec(t.Context(), `UPDATE tenant.organizations SET quota_clusters = DEFAULT WHERE id = $1`, orgID)
	require.NoError(t, err)

	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+env.createAuthnToken(t, userID))
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	return &quotaEnv{
		env:    env,
		orgID:  orgID,
		client: organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL),
		ctx:    ctx,
	}
}

func (q *quotaEnv) setClusterQuota(t *testing.T, quota int) {
	t.Helper()
	_, err := q.env.adminPool.Exec(t.Context(), `UPDATE tenant.organizations SET quota_clusters = $2 WHERE id = $1`, q.orgID, quota)
	require.NoError(t, err)
}

func (q *quotaEnv) createCluster(name string) (string, error) {
	res, err := q.client.CreateCluster(q.ctx, organizationv1.CreateClusterRequest_builder{
		Name:              name,
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build())
	if err != nil {
		return "", err
	}
	return res.GetClusterId(), nil
}

func requireResourceExhausted(t *testing.T, err error, message string) {
	t.Helper()
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeResourceExhausted, connectErr.Code())
	assert.Equal(t, message, connectErr.Message())
}

func Test_Cluster_Create_QuotaDefaultsToOne(t *testing.T) {
	t.Parallel()
	q := newQuotaEnv(t)

	_, err := q.createCluster("first")
	require.NoError(t, err)

	_, err = q.createCluster("second")
	requireResourceExhausted(t, err, "the organization has reached its quota of 1 cluster(s)")

	var clusters int
	err = q.env.adminPool.QueryRow(t.Context(), `SELECT count(*) FROM tenant.clusters WHERE organization_id = $1`, q.orgID).Scan(&clusters)
	require.NoError(t, err)
	assert.Equal(t, 1, clusters, "the refused cluster is not created")

	// Raising the quota lets it through.
	q.setClusterQuota(t, 2)
	_, err = q.createCluster("second")
	require.NoError(t, err)
}

func Test_Cluster_Create_QuotaZeroStopsAll(t *testing.T) {
	t.Parallel()
	q := newQuotaEnv(t)
	q.setClusterQuota(t, 0)

	_, err := q.createCluster("first")
	requireResourceExhausted(t, err, "the organization has reached its quota of 0 cluster(s)")
}

// A deleted cluster still holds machines until Gardener has torn its shoot
// down, so it keeps counting until then; the same rule the name check uses.
func Test_Cluster_Create_DeletedClusterCountsUntilTornDown(t *testing.T) {
	t.Parallel()
	q := newQuotaEnv(t)

	firstID, err := q.createCluster("first")
	require.NoError(t, err)

	_, err = q.client.DeleteCluster(q.ctx, organizationv1.DeleteClusterRequest_builder{ClusterId: firstID}.Build())
	require.NoError(t, err)

	_, err = q.createCluster("second")
	requireResourceExhausted(t, err, "the organization has reached its quota of 1 cluster(s)")

	_, err = q.env.adminPool.Exec(t.Context(), `UPDATE tenant.clusters SET shoot_status = 'deleted' WHERE id = $1`, firstID)
	require.NoError(t, err)

	_, err = q.createCluster("second")
	require.NoError(t, err)
}

// Lowering the quota below what the organization runs changes nothing about
// its clusters; it only stops the next one.
func Test_Cluster_Create_LoweredQuotaKeepsExistingClusters(t *testing.T) {
	t.Parallel()
	q := newQuotaEnv(t)
	q.setClusterQuota(t, 3)

	for _, name := range []string{"a", "b", "c"} {
		_, err := q.createCluster(name)
		require.NoError(t, err)
	}

	q.setClusterQuota(t, 1)

	res, err := q.client.ListClusters(q.ctx, organizationv1.ListClustersRequest_builder{}.Build())
	require.NoError(t, err)
	assert.Len(t, res.GetClusters(), 3)

	_, err = q.createCluster("d")
	requireResourceExhausted(t, err, "the organization has reached its quota of 1 cluster(s)")
}
