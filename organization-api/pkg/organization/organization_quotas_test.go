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

func Test_Organization_Quotas_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := organizationv1connect.NewOrganizationServiceClient(env.server.Client(), env.server.URL)

	_, err := client.GetOrganizationQuotas(context.Background(), organizationv1.GetOrganizationQuotasRequest_builder{}.Build())

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

// The page shows what funops set and what counts against it: the same numbers
// the triggers and `funops organization quota` work with.
func Test_Organization_Quotas(t *testing.T) {
	t.Parallel()
	q := newQuotaEnv(t)
	q.setClusterQuota(t, 3)
	q.setNodeQuota(t, offeringN1Standard1, 10)

	clusterID, err := q.createCluster("c")
	require.NoError(t, err)
	_, err = q.createNodePool(clusterID, "a", "n1-standard-1", 4)
	require.NoError(t, err)
	_, err = q.createNodePool(clusterID, "b", "n1-standard-1", 2)
	require.NoError(t, err)

	client := organizationv1connect.NewOrganizationServiceClient(q.env.server.Client(), q.env.server.URL)
	res, err := client.GetOrganizationQuotas(q.ctx, organizationv1.GetOrganizationQuotasRequest_builder{}.Build())
	require.NoError(t, err)

	assert.Equal(t, int32(3), res.GetClusters().GetQuota())
	assert.Equal(t, int32(1), res.GetClusters().GetInUse())

	require.Len(t, res.GetNodes(), 1, "only offerings with a quota or with nodes in use")
	node := res.GetNodes()[0]
	assert.Equal(t, "eu-west-1", node.GetRegion())
	assert.Equal(t, "n1-standard-1", node.GetMachineType())
	assert.Equal(t, int32(10), node.GetQuota())
	assert.Equal(t, int32(6), node.GetInUse(), "the maxima add up")

	// A second offering shows up once it has a quota, even with nothing in use.
	q.setNodeQuota(t, offeringN1Standard2, 0)
	res, err = client.GetOrganizationQuotas(q.ctx, organizationv1.GetOrganizationQuotasRequest_builder{}.Build())
	require.NoError(t, err)
	require.Len(t, res.GetNodes(), 2)
	assert.Equal(t, "n1-standard-2", res.GetNodes()[1].GetMachineType())
	assert.Equal(t, int32(0), res.GetNodes()[1].GetQuota())
}

// One organization cannot read another's quotas: the policy scopes the rows,
// and the cluster quota is read for the organization of the request only.
func Test_Organization_Quotas_IsolatedPerOrganization(t *testing.T) {
	t.Parallel()

	richID := uuid.New()
	poorID := uuid.New()
	userID := uuid.New()
	env := newTestAPI(t,
		WithOrganization(richID, "rich"),
		WithOrganization(poorID, "poor"),
		WithUser(&UserArgs{ID: userID, Name: "both", OrgIDs: []uuid.UUID{richID, poorID}}),
	)
	_, err := env.adminPool.Exec(t.Context(), `UPDATE tenant.organizations SET quota_clusters = 7 WHERE id = $1`, richID)
	require.NoError(t, err)
	_, err = env.adminPool.Exec(t.Context(), `
		INSERT INTO tenant.organization_machine_quotas (organization_id, region_machine_type_id, max_nodes)
		VALUES ($1, $2, 10)`, richID, offeringN1Standard1)
	require.NoError(t, err)

	client := organizationv1connect.NewOrganizationServiceClient(env.server.Client(), env.server.URL)
	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+env.createAuthnToken(t, userID))
	callInfo.RequestHeader().Set("Fun-Organization", poorID.String())

	res, err := client.GetOrganizationQuotas(ctx, organizationv1.GetOrganizationQuotasRequest_builder{}.Build())
	require.NoError(t, err)
	assert.Equal(t, int32(100), res.GetClusters().GetQuota(), "the harness default for poor, not rich's 7")
	assert.Empty(t, res.GetNodes(), "rich's node quota is not poor's")
}
