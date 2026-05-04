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

func Test_ProjectLimits_Update_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)

	_, err := client.UpdateProjectLimits(context.Background(), connect.NewRequest(
		organizationv1.UpdateProjectLimitsRequest_builder{ProjectId: uuid.New().String()}.Build(),
	))

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_ProjectLimits_Update(t *testing.T) {
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

	createClusterReq := connect.NewRequest(organizationv1.CreateClusterRequest_builder{
		Name: "test-cluster", Region: "eu-west-1", KubernetesVersion: "1.28",
	}.Build())
	createClusterReq.Header().Set("Authorization", "Bearer "+token)
	createClusterReq.Header().Set("Fun-Organization", orgID.String())
	clusterRes, err := clusterClient.CreateCluster(context.Background(), createClusterReq)
	require.NoError(t, err)

	createProjectReq := connect.NewRequest(organizationv1.CreateProjectRequest_builder{
		ClusterId: clusterRes.Msg.GetClusterId(), Name: "test-project",
	}.Build())
	createProjectReq.Header().Set("Authorization", "Bearer "+token)
	createProjectReq.Header().Set("Fun-Organization", orgID.String())
	projectRes, err := projectClient.CreateProject(context.Background(), createProjectReq)
	require.NoError(t, err)
	projectID := projectRes.Msg.GetProjectId()

	updateReq := connect.NewRequest(organizationv1.UpdateProjectLimitsRequest_builder{
		ProjectId:              projectID,
		DefaultMemoryRequestMi: proto.Int32(128),
		DefaultMemoryLimitMi:   proto.Int32(256),
		DefaultCpuRequestM:     proto.Int32(100),
		DefaultCpuLimitM:       proto.Int32(500),
	}.Build())
	updateReq.Header().Set("Authorization", "Bearer "+token)
	updateReq.Header().Set("Fun-Organization", orgID.String())

	_, err = projectClient.UpdateProjectLimits(context.Background(), updateReq)
	require.NoError(t, err)

	getReq := connect.NewRequest(organizationv1.GetProjectLimitsRequest_builder{ProjectId: projectID}.Build())
	getReq.Header().Set("Authorization", "Bearer "+token)
	getReq.Header().Set("Fun-Organization", orgID.String())

	getRes, err := projectClient.GetProjectLimits(context.Background(), getReq)
	require.NoError(t, err)

	limits := getRes.Msg.GetLimits()
	require.NotNil(t, limits)
	assert.EqualValues(t, 128, limits.GetDefaultMemoryRequestMi())
	assert.EqualValues(t, 256, limits.GetDefaultMemoryLimitMi())
	assert.EqualValues(t, 100, limits.GetDefaultCpuRequestM())
	assert.EqualValues(t, 500, limits.GetDefaultCpuLimitM())
}

func Test_ProjectLimits_Update_Overwrites(t *testing.T) {
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

	createClusterReq := connect.NewRequest(organizationv1.CreateClusterRequest_builder{
		Name: "test-cluster", Region: "eu-west-1", KubernetesVersion: "1.28",
	}.Build())
	createClusterReq.Header().Set("Authorization", "Bearer "+token)
	createClusterReq.Header().Set("Fun-Organization", orgID.String())
	clusterRes, err := clusterClient.CreateCluster(context.Background(), createClusterReq)
	require.NoError(t, err)

	createProjectReq := connect.NewRequest(organizationv1.CreateProjectRequest_builder{
		ClusterId: clusterRes.Msg.GetClusterId(), Name: "test-project",
	}.Build())
	createProjectReq.Header().Set("Authorization", "Bearer "+token)
	createProjectReq.Header().Set("Fun-Organization", orgID.String())
	projectRes, err := projectClient.CreateProject(context.Background(), createProjectReq)
	require.NoError(t, err)
	projectID := projectRes.Msg.GetProjectId()

	firstUpdate := connect.NewRequest(organizationv1.UpdateProjectLimitsRequest_builder{
		ProjectId:            projectID,
		DefaultMemoryLimitMi: proto.Int32(256),
	}.Build())
	firstUpdate.Header().Set("Authorization", "Bearer "+token)
	firstUpdate.Header().Set("Fun-Organization", orgID.String())
	_, err = projectClient.UpdateProjectLimits(context.Background(), firstUpdate)
	require.NoError(t, err)

	secondUpdate := connect.NewRequest(organizationv1.UpdateProjectLimitsRequest_builder{
		ProjectId:            projectID,
		DefaultMemoryLimitMi: proto.Int32(512),
	}.Build())
	secondUpdate.Header().Set("Authorization", "Bearer "+token)
	secondUpdate.Header().Set("Fun-Organization", orgID.String())
	_, err = projectClient.UpdateProjectLimits(context.Background(), secondUpdate)
	require.NoError(t, err)

	getReq := connect.NewRequest(organizationv1.GetProjectLimitsRequest_builder{ProjectId: projectID}.Build())
	getReq.Header().Set("Authorization", "Bearer "+token)
	getReq.Header().Set("Fun-Organization", orgID.String())
	getRes, err := projectClient.GetProjectLimits(context.Background(), getReq)
	require.NoError(t, err)

	assert.EqualValues(t, 512, getRes.Msg.GetLimits().GetDefaultMemoryLimitMi())
}

func Test_ProjectLimits_Update_IsolatedBetweenProjects(t *testing.T) {
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

	updateReq := connect.NewRequest(organizationv1.UpdateProjectLimitsRequest_builder{
		ProjectId:            project1Res.Msg.GetProjectId(),
		DefaultMemoryLimitMi: proto.Int32(256),
	}.Build())
	updateReq.Header().Set("Authorization", "Bearer "+token)
	updateReq.Header().Set("Fun-Organization", orgID.String())
	_, err = projectClient.UpdateProjectLimits(context.Background(), updateReq)
	require.NoError(t, err)

	// project2 should see no limits
	getReq := connect.NewRequest(organizationv1.GetProjectLimitsRequest_builder{
		ProjectId: project2Res.Msg.GetProjectId(),
	}.Build())
	getReq.Header().Set("Authorization", "Bearer "+token)
	getReq.Header().Set("Fun-Organization", orgID.String())
	getRes, err := projectClient.GetProjectLimits(context.Background(), getReq)
	require.NoError(t, err)

	assert.False(t, getRes.Msg.GetLimits().HasDefaultMemoryLimitMi())
}

func Test_ProjectLimits_Update_IsolatedBetweenOrgs(t *testing.T) {
	t.Parallel()

	org1ID := uuid.New()
	org2ID := uuid.New()
	user1ID := uuid.New()
	user2ID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(org1ID, "org-one"),
		WithOrganization(org2ID, "org-two"),
		WithUser(&UserArgs{ID: user1ID, Name: "user-one", OrgIDs: []uuid.UUID{org1ID}}),
		WithUser(&UserArgs{ID: user2ID, Name: "user-two", OrgIDs: []uuid.UUID{org2ID}}),
	)

	token1 := env.createAuthnToken(t, user1ID)
	token2 := env.createAuthnToken(t, user2ID)
	clusterClient := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)
	projectClient := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)

	// Create a cluster and project in org1
	createClusterReq := connect.NewRequest(organizationv1.CreateClusterRequest_builder{
		Name: "test-cluster", Region: "eu-west-1", KubernetesVersion: "1.28",
	}.Build())
	createClusterReq.Header().Set("Authorization", "Bearer "+token1)
	createClusterReq.Header().Set("Fun-Organization", org1ID.String())
	clusterRes, err := clusterClient.CreateCluster(context.Background(), createClusterReq)
	require.NoError(t, err)

	createProjectReq := connect.NewRequest(organizationv1.CreateProjectRequest_builder{
		ClusterId: clusterRes.Msg.GetClusterId(), Name: "test-project",
	}.Build())
	createProjectReq.Header().Set("Authorization", "Bearer "+token1)
	createProjectReq.Header().Set("Fun-Organization", org1ID.String())
	projectRes, err := projectClient.CreateProject(context.Background(), createProjectReq)
	require.NoError(t, err)
	projectID := projectRes.Msg.GetProjectId()

	// Set limits on org1's project
	updateReq := connect.NewRequest(organizationv1.UpdateProjectLimitsRequest_builder{
		ProjectId:            projectID,
		DefaultMemoryLimitMi: proto.Int32(256),
	}.Build())
	updateReq.Header().Set("Authorization", "Bearer "+token1)
	updateReq.Header().Set("Fun-Organization", org1ID.String())
	_, err = projectClient.UpdateProjectLimits(context.Background(), updateReq)
	require.NoError(t, err)

	// user2 (org2) should not see org1's project limits
	getReq := connect.NewRequest(organizationv1.GetProjectLimitsRequest_builder{ProjectId: projectID}.Build())
	getReq.Header().Set("Authorization", "Bearer "+token2)
	getReq.Header().Set("Fun-Organization", org2ID.String())
	getRes, err := projectClient.GetProjectLimits(context.Background(), getReq)
	require.NoError(t, err)
	assert.False(t, getRes.Msg.GetLimits().HasDefaultMemoryLimitMi())
}

func Test_ProjectLimits_Update_MemoryLimitLessThanRequest(t *testing.T) {
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

	createClusterReq := connect.NewRequest(organizationv1.CreateClusterRequest_builder{
		Name: "test-cluster", Region: "eu-west-1", KubernetesVersion: "1.28",
	}.Build())
	createClusterReq.Header().Set("Authorization", "Bearer "+token)
	createClusterReq.Header().Set("Fun-Organization", orgID.String())
	clusterRes, err := clusterClient.CreateCluster(context.Background(), createClusterReq)
	require.NoError(t, err)

	createProjectReq := connect.NewRequest(organizationv1.CreateProjectRequest_builder{
		ClusterId: clusterRes.Msg.GetClusterId(), Name: "test-project",
	}.Build())
	createProjectReq.Header().Set("Authorization", "Bearer "+token)
	createProjectReq.Header().Set("Fun-Organization", orgID.String())
	projectRes, err := projectClient.CreateProject(context.Background(), createProjectReq)
	require.NoError(t, err)

	req := connect.NewRequest(organizationv1.UpdateProjectLimitsRequest_builder{
		ProjectId:              projectRes.Msg.GetProjectId(),
		DefaultMemoryRequestMi: proto.Int32(256),
		DefaultMemoryLimitMi:   proto.Int32(128),
	}.Build())
	req.Header().Set("Authorization", "Bearer "+token)
	req.Header().Set("Fun-Organization", orgID.String())

	_, err = projectClient.UpdateProjectLimits(context.Background(), req)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func Test_ProjectLimits_Update_CpuLimitLessThanRequest(t *testing.T) {
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

	createClusterReq := connect.NewRequest(organizationv1.CreateClusterRequest_builder{
		Name: "test-cluster", Region: "eu-west-1", KubernetesVersion: "1.28",
	}.Build())
	createClusterReq.Header().Set("Authorization", "Bearer "+token)
	createClusterReq.Header().Set("Fun-Organization", orgID.String())
	clusterRes, err := clusterClient.CreateCluster(context.Background(), createClusterReq)
	require.NoError(t, err)

	createProjectReq := connect.NewRequest(organizationv1.CreateProjectRequest_builder{
		ClusterId: clusterRes.Msg.GetClusterId(), Name: "test-project",
	}.Build())
	createProjectReq.Header().Set("Authorization", "Bearer "+token)
	createProjectReq.Header().Set("Fun-Organization", orgID.String())
	projectRes, err := projectClient.CreateProject(context.Background(), createProjectReq)
	require.NoError(t, err)

	req := connect.NewRequest(organizationv1.UpdateProjectLimitsRequest_builder{
		ProjectId:          projectRes.Msg.GetProjectId(),
		DefaultCpuRequestM: proto.Int32(500),
		DefaultCpuLimitM:   proto.Int32(100),
	}.Build())
	req.Header().Set("Authorization", "Bearer "+token)
	req.Header().Set("Fun-Organization", orgID.String())

	_, err = projectClient.UpdateProjectLimits(context.Background(), req)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}
