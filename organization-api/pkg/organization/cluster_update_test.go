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

	_, err := client.UpdateCluster(context.Background(), connect.NewRequest(organizationv1.UpdateClusterRequest_builder{
		ClusterId:         uuid.New().String(),
		KubernetesVersion: proto.String("1.29"),
	}.Build()))

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

	createReq := connect.NewRequest(organizationv1.CreateClusterRequest_builder{
		Name:              "test-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build())
	createReq.Header().Set("Authorization", "Bearer "+token)
	createReq.Header().Set("Fun-Organization", orgID.String())

	createRes, err := client.CreateCluster(context.Background(), createReq)
	require.NoError(t, err)

	clusterID := createRes.Msg.GetClusterId()

	updateReq := connect.NewRequest(organizationv1.UpdateClusterRequest_builder{
		ClusterId:         clusterID,
		KubernetesVersion: proto.String("1.29"),
	}.Build())
	updateReq.Header().Set("Authorization", "Bearer "+token)
	updateReq.Header().Set("Fun-Organization", orgID.String())

	_, err = client.UpdateCluster(context.Background(), updateReq)
	require.NoError(t, err)

	getReq := connect.NewRequest(organizationv1.GetClusterRequest_builder{
		ClusterId: clusterID,
	}.Build())
	getReq.Header().Set("Authorization", "Bearer "+token)
	getReq.Header().Set("Fun-Organization", orgID.String())

	getRes, err := client.GetCluster(context.Background(), getReq)
	require.NoError(t, err)
	assert.Equal(t, "1.29", getRes.Msg.GetCluster().GetKubernetesVersion())
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

	req := connect.NewRequest(organizationv1.UpdateClusterRequest_builder{
		ClusterId:         uuid.New().String(),
		KubernetesVersion: proto.String("1.29"),
	}.Build())
	req.Header().Set("Authorization", "Bearer "+token)
	req.Header().Set("Fun-Organization", orgID.String())

	_, err := client.UpdateCluster(context.Background(), req)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}
