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

func Test_Cluster_List_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)

	client := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)

	_, err := client.ListClusters(context.Background(), connect.NewRequest(&organizationv1.ListClustersRequest{}))

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_Cluster_List(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(userID, "test-user", "", nil, []uuid.UUID{orgID}),
	)

	token := env.createAuthnToken(t, userID)

	client := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)

	createReq := connect.NewRequest(&organizationv1.CreateClusterRequest{
		Name:              "test-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	})
	createReq.Header().Set("Authorization", "Bearer "+token)
	createReq.Header().Set("Fun-Organization", orgID.String())

	_, err := client.CreateCluster(context.Background(), createReq)
	require.NoError(t, err)

	listReq := connect.NewRequest(&organizationv1.ListClustersRequest{})
	listReq.Header().Set("Authorization", "Bearer "+token)
	listReq.Header().Set("Fun-Organization", orgID.String())

	res, err := client.ListClusters(context.Background(), listReq)
	require.NoError(t, err)
	require.Len(t, res.Msg.Clusters, 1)

	cluster := res.Msg.Clusters[0]
	assert.Equal(t, "test-cluster", cluster.Name)
	assert.Equal(t, "eu-west-1", cluster.Region)
	// TODO: kubernetes version missing in cluster?
	assert.Equal(t, organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING, cluster.Status)
}
