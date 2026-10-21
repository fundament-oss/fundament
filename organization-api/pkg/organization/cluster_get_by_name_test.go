package organization_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

// A cluster read by name is the same cluster as read by ID, upgrading included.
func Test_Cluster_GetByName_MatchesGetByID(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()
	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{ID: userID, Name: "test-user", OrgIDs: []uuid.UUID{orgID}}),
	)
	token := env.createAuthnToken(t, userID)
	client := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)

	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	createRes, err := client.CreateCluster(ctx, organizationv1.CreateClusterRequest_builder{
		Name: "upgrading", Region: "eu-west-1", KubernetesVersion: "1.28",
	}.Build())
	require.NoError(t, err)
	clusterID := createRes.GetClusterId()

	// Gardener runs the cluster and rolls out an update fundament pushed.
	_, err = env.adminPool.Exec(t.Context(),
		`UPDATE tenant.clusters SET shoot_status = 'ready', shoot_updating = true WHERE id = $1`, clusterID)
	require.NoError(t, err)

	byID, err := client.GetCluster(ctx, organizationv1.GetClusterRequest_builder{ClusterId: clusterID}.Build())
	require.NoError(t, err)
	byName, err := client.GetClusterByName(ctx, organizationv1.GetClusterByNameRequest_builder{Name: "upgrading"}.Build())
	require.NoError(t, err)

	assert.Equal(t, organizationv1.ClusterStatus_CLUSTER_STATUS_UPGRADING, byName.GetCluster().GetStatus())
	assert.True(t, proto.Equal(byID.GetCluster(), byName.GetCluster()), "by name: %v\nby ID:   %v", byName.GetCluster(), byID.GetCluster())
}
