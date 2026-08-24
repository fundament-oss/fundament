package organization_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Cluster_Get_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)

	client := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)

	_, err := client.GetCluster(context.Background(), organizationv1.GetClusterRequest_builder{
		ClusterId: uuid.New().String(),
	}.Build())

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_Cluster_Get(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
	)

	token := env.createAuthnToken(t, userID)

	client := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)

	createReq := organizationv1.CreateClusterRequest_builder{
		Name:              "test-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build()
	createCtx, createCallInfo := connect.NewClientContext(context.Background())
	createCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	createRes, err := client.CreateCluster(createCtx, createReq)
	require.NoError(t, err)

	clusterID := createRes.GetClusterId()

	getReq := organizationv1.GetClusterRequest_builder{
		ClusterId: clusterID,
	}.Build()
	getCtx, getCallInfo := connect.NewClientContext(context.Background())
	getCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	getCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	res, err := client.GetCluster(getCtx, getReq)
	require.NoError(t, err)

	cluster := res.GetCluster()
	assert.Equal(t, clusterID, cluster.GetId())
	assert.Equal(t, "test-cluster", cluster.GetName())
	assert.Equal(t, "eu-west-1", cluster.GetRegion())
	assert.Equal(t, organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING, cluster.GetStatus())
}

func Test_Cluster_Get_OtherOrg(t *testing.T) {
	t.Parallel()

	orgAID := uuid.New()
	orgBID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgAID, "org-a"),
		WithOrganization(orgBID, "org-b"),
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			OrgIDs: []uuid.UUID{orgAID, orgBID},
		}),
	)

	token := env.createAuthnToken(t, userID)

	client := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)

	createReq := organizationv1.CreateClusterRequest_builder{
		Name:              "org-a-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build()
	createCtx, createCallInfo := connect.NewClientContext(context.Background())
	createCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createCallInfo.RequestHeader().Set("Fun-Organization", orgAID.String())

	createRes, err := client.CreateCluster(createCtx, createReq)
	require.NoError(t, err)

	clusterID := createRes.GetClusterId()

	getReq := organizationv1.GetClusterRequest_builder{
		ClusterId: clusterID,
	}.Build()
	getCtx, getCallInfo := connect.NewClientContext(context.Background())
	getCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	getCallInfo.RequestHeader().Set("Fun-Organization", orgBID.String())

	_, err = client.GetCluster(getCtx, getReq)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}
