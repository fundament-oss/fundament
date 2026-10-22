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

func Test_Cluster_Delete_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)

	_, err := client.DeleteCluster(context.Background(), organizationv1.DeleteClusterRequest_builder{
		ClusterId: uuid.New().String(),
	}.Build())

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_Cluster_Delete(t *testing.T) {
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

	deleteReq := organizationv1.DeleteClusterRequest_builder{
		ClusterId: clusterID,
	}.Build()
	deleteCtx, deleteCallInfo := connect.NewClientContext(context.Background())
	deleteCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	deleteCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	_, err = client.DeleteCluster(deleteCtx, deleteReq)
	require.NoError(t, err)

	// Soft delete: the cluster must no longer be visible via GetCluster.
	getReq := organizationv1.GetClusterRequest_builder{
		ClusterId: clusterID,
	}.Build()
	getCtx, getCallInfo := connect.NewClientContext(context.Background())
	getCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	getCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	_, err = client.GetCluster(getCtx, getReq)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())

	// Deleting the same cluster again must return not found.
	_, err = client.DeleteCluster(deleteCtx, deleteReq)
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// Covers the API only. The console flow on top of it (the blocked dialog that
// lists the projects) has no e2e test yet: the e2e suite has no cluster,
// project or namespace clients or steps to build one on, and a test written
// without running it against the full stack would only look like coverage.
func Test_Cluster_Delete_WithProjects(t *testing.T) {
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
	clusterClient := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)
	projectClient := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)

	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	createRes, err := clusterClient.CreateCluster(ctx, organizationv1.CreateClusterRequest_builder{
		Name:              "test-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build())
	require.NoError(t, err)

	clusterID := createRes.GetClusterId()

	projectRes, err := projectClient.CreateProject(ctx, organizationv1.CreateProjectRequest_builder{
		ClusterId: clusterID,
		Name:      "team-a",
	}.Build())
	require.NoError(t, err)

	deleteReq := organizationv1.DeleteClusterRequest_builder{
		ClusterId: clusterID,
	}.Build()

	// The project blocks the deletion, and the error names it.
	_, err = clusterClient.DeleteCluster(ctx, deleteReq)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
	assert.Contains(t, connectErr.Message(), `"team-a"`)

	_, err = projectClient.DeleteProject(ctx, organizationv1.DeleteProjectRequest_builder{
		ProjectId: projectRes.GetProjectId(),
	}.Build())
	require.NoError(t, err)

	// Without its project, the cluster can be deleted.
	_, err = clusterClient.DeleteCluster(ctx, deleteReq)
	require.NoError(t, err)
}
