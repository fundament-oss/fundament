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

// Project names are unique per cluster, so a name alone can match projects on
// several clusters.
func Test_Project_GetByName_SameNameOnTwoClusters(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{ID: userID, Name: "test-user", OrgIDs: []uuid.UUID{orgID}}),
	)

	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+env.createAuthnToken(t, userID))
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	clusterClient := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)
	client := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)

	projectIDs := map[string]string{} // cluster ID to project ID
	for _, name := range []string{"cluster-a", "cluster-b"} {
		cluster, err := clusterClient.CreateCluster(ctx, organizationv1.CreateClusterRequest_builder{
			Name:              name,
			Region:            "eu-west-1",
			KubernetesVersion: "1.28",
		}.Build())
		require.NoError(t, err)
		project, err := client.CreateProject(ctx, organizationv1.CreateProjectRequest_builder{
			ClusterId: cluster.GetClusterId(),
			Name:      "shared",
		}.Build())
		require.NoError(t, err)
		projectIDs[cluster.GetClusterId()] = project.GetProjectId()
	}

	_, err := client.GetProjectByName(ctx, organizationv1.GetProjectByNameRequest_builder{Name: "shared"}.Build())
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))

	for clusterID, projectID := range projectIDs {
		resp, err := client.GetProjectByName(ctx, organizationv1.GetProjectByNameRequest_builder{
			Name:      "shared",
			ClusterId: &clusterID,
		}.Build())
		require.NoError(t, err)
		assert.Equal(t, projectID, resp.GetProject().GetId())
		assert.Equal(t, clusterID, resp.GetProject().GetClusterId())
	}

	_, err = client.GetProjectByName(ctx, organizationv1.GetProjectByNameRequest_builder{Name: "missing"}.Build())
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}
