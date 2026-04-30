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

func Test_Namespace_Create_DuplicateNameInCluster(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{ID: userID, Name: "test-user", OrgIDs: []uuid.UUID{orgID}}),
	)

	token := env.createAuthnToken(t, userID)
	clusterClient := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)
	projectClient := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)
	namespaceClient := organizationv1connect.NewNamespaceServiceClient(env.server.Client(), env.server.URL)

	createClusterReq := connect.NewRequest(organizationv1.CreateClusterRequest_builder{
		Name: "test-cluster", Region: "eu-west-1", KubernetesVersion: "1.28",
	}.Build())
	createClusterReq.Header().Set("Authorization", "Bearer "+token)
	createClusterReq.Header().Set("Fun-Organization", orgID.String())
	clusterRes, err := clusterClient.CreateCluster(context.Background(), createClusterReq)
	require.NoError(t, err)
	clusterID := clusterRes.Msg.GetClusterId()

	createProject1Req := connect.NewRequest(organizationv1.CreateProjectRequest_builder{
		ClusterId: clusterID, Name: "project-one",
	}.Build())
	createProject1Req.Header().Set("Authorization", "Bearer "+token)
	createProject1Req.Header().Set("Fun-Organization", orgID.String())
	project1Res, err := projectClient.CreateProject(context.Background(), createProject1Req)
	require.NoError(t, err)

	createProject2Req := connect.NewRequest(organizationv1.CreateProjectRequest_builder{
		ClusterId: clusterID, Name: "project-two",
	}.Build())
	createProject2Req.Header().Set("Authorization", "Bearer "+token)
	createProject2Req.Header().Set("Fun-Organization", orgID.String())
	project2Res, err := projectClient.CreateProject(context.Background(), createProject2Req)
	require.NoError(t, err)

	// Create namespace "duplicate" in project one — should succeed
	ns1Req := connect.NewRequest(organizationv1.CreateNamespaceRequest_builder{
		ProjectId: project1Res.Msg.GetProjectId(), Name: "duplicate",
	}.Build())
	ns1Req.Header().Set("Authorization", "Bearer "+token)
	ns1Req.Header().Set("Fun-Organization", orgID.String())
	_, err = namespaceClient.CreateNamespace(context.Background(), ns1Req)
	require.NoError(t, err)

	// Create namespace "duplicate" in project two (same cluster) — must fail
	ns2Req := connect.NewRequest(organizationv1.CreateNamespaceRequest_builder{
		ProjectId: project2Res.Msg.GetProjectId(), Name: "duplicate",
	}.Build())
	ns2Req.Header().Set("Authorization", "Bearer "+token)
	ns2Req.Header().Set("Fun-Organization", orgID.String())
	_, err = namespaceClient.CreateNamespace(context.Background(), ns2Req)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeAlreadyExists, connectErr.Code())
}

func Test_Namespace_Create_SameNameDifferentClusters(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{ID: userID, Name: "test-user", OrgIDs: []uuid.UUID{orgID}}),
	)

	token := env.createAuthnToken(t, userID)
	clusterClient := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)
	projectClient := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)
	namespaceClient := organizationv1connect.NewNamespaceServiceClient(env.server.Client(), env.server.URL)

	createCluster := func(name string) string {
		req := connect.NewRequest(organizationv1.CreateClusterRequest_builder{
			Name: name, Region: "eu-west-1", KubernetesVersion: "1.28",
		}.Build())
		req.Header().Set("Authorization", "Bearer "+token)
		req.Header().Set("Fun-Organization", orgID.String())
		res, err := clusterClient.CreateCluster(context.Background(), req)
		require.NoError(t, err)
		return res.Msg.GetClusterId()
	}

	createProject := func(clusterID, name string) string {
		req := connect.NewRequest(organizationv1.CreateProjectRequest_builder{
			ClusterId: clusterID, Name: name,
		}.Build())
		req.Header().Set("Authorization", "Bearer "+token)
		req.Header().Set("Fun-Organization", orgID.String())
		res, err := projectClient.CreateProject(context.Background(), req)
		require.NoError(t, err)
		return res.Msg.GetProjectId()
	}

	cluster1ID := createCluster("cluster-one")
	cluster2ID := createCluster("cluster-two")
	project1ID := createProject(cluster1ID, "project-one")
	project2ID := createProject(cluster2ID, "project-two")

	// Create namespace "shared" in cluster one — must succeed
	ns1Req := connect.NewRequest(organizationv1.CreateNamespaceRequest_builder{
		ProjectId: project1ID, Name: "shared",
	}.Build())
	ns1Req.Header().Set("Authorization", "Bearer "+token)
	ns1Req.Header().Set("Fun-Organization", orgID.String())
	_, err := namespaceClient.CreateNamespace(context.Background(), ns1Req)
	require.NoError(t, err)

	// Create namespace "shared" in cluster two — must also succeed
	ns2Req := connect.NewRequest(organizationv1.CreateNamespaceRequest_builder{
		ProjectId: project2ID, Name: "shared",
	}.Build())
	ns2Req.Header().Set("Authorization", "Bearer "+token)
	ns2Req.Header().Set("Fun-Organization", orgID.String())
	_, err = namespaceClient.CreateNamespace(context.Background(), ns2Req)
	require.NoError(t, err)
}
