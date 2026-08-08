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
	"google.golang.org/protobuf/proto"
)

func Test_Cluster_Update_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)

	_, err := client.UpdateCluster(context.Background(), organizationv1.UpdateClusterRequest_builder{
		ClusterId:         uuid.New().String(),
		KubernetesVersion: proto.String("1.29"),
	}.Build())

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_Cluster_Update(t *testing.T) {
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

	updateReq := organizationv1.UpdateClusterRequest_builder{
		ClusterId:         clusterID,
		KubernetesVersion: proto.String("1.29"),
	}.Build()
	updateCtx, updateCallInfo := connect.NewClientContext(context.Background())
	updateCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	updateCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	_, err = client.UpdateCluster(updateCtx, updateReq)
	require.NoError(t, err)

	getReq := organizationv1.GetClusterRequest_builder{
		ClusterId: clusterID,
	}.Build()
	getCtx, getCallInfo := connect.NewClientContext(context.Background())
	getCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	getCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	getRes, err := client.GetCluster(getCtx, getReq)
	require.NoError(t, err)
	assert.Equal(t, "1.29", getRes.GetCluster().GetKubernetesVersion())
}

func Test_Cluster_Update_NotFound(t *testing.T) {
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

	req := organizationv1.UpdateClusterRequest_builder{
		ClusterId:         uuid.New().String(),
		KubernetesVersion: proto.String("1.29"),
	}.Build()
	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	_, err := client.UpdateCluster(ctx, req)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}
