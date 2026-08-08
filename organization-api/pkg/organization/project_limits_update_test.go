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

	_, err := client.UpdateProjectLimits(context.Background(),
		organizationv1.UpdateProjectLimitsRequest_builder{ProjectId: uuid.New().String()}.Build(),
	)

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

	createClusterReq := organizationv1.CreateClusterRequest_builder{
		Name: "test-cluster", Region: "eu-west-1", KubernetesVersion: "1.28",
	}.Build()
	createClusterCtx, createClusterCallInfo := connect.NewClientContext(context.Background())
	createClusterCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createClusterCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())
	clusterRes, err := clusterClient.CreateCluster(createClusterCtx, createClusterReq)
	require.NoError(t, err)

	createProjectReq := organizationv1.CreateProjectRequest_builder{
		ClusterId: clusterRes.GetClusterId(), Name: "test-project",
	}.Build()
	createProjectCtx, createProjectCallInfo := connect.NewClientContext(context.Background())
	createProjectCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createProjectCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())
	projectRes, err := projectClient.CreateProject(createProjectCtx, createProjectReq)
	require.NoError(t, err)
	projectID := projectRes.GetProjectId()

	updateReq := organizationv1.UpdateProjectLimitsRequest_builder{
		ProjectId:              projectID,
		DefaultMemoryRequestMi: proto.Int32(128),
		DefaultMemoryLimitMi:   proto.Int32(256),
		DefaultCpuRequestM:     proto.Int32(100),
		DefaultCpuLimitM:       proto.Int32(500),
	}.Build()
	updateCtx, updateCallInfo := connect.NewClientContext(context.Background())
	updateCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	updateCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	_, err = projectClient.UpdateProjectLimits(updateCtx, updateReq)
	require.NoError(t, err)

	getReq := organizationv1.GetProjectLimitsRequest_builder{ProjectId: projectID}.Build()
	getCtx, getCallInfo := connect.NewClientContext(context.Background())
	getCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	getCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	getRes, err := projectClient.GetProjectLimits(getCtx, getReq)
	require.NoError(t, err)

	limits := getRes.GetLimits()
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

	createClusterReq := organizationv1.CreateClusterRequest_builder{
		Name: "test-cluster", Region: "eu-west-1", KubernetesVersion: "1.28",
	}.Build()
	createClusterCtx, createClusterCallInfo := connect.NewClientContext(context.Background())
	createClusterCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createClusterCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())
	clusterRes, err := clusterClient.CreateCluster(createClusterCtx, createClusterReq)
	require.NoError(t, err)

	createProjectReq := organizationv1.CreateProjectRequest_builder{
		ClusterId: clusterRes.GetClusterId(), Name: "test-project",
	}.Build()
	createProjectCtx, createProjectCallInfo := connect.NewClientContext(context.Background())
	createProjectCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createProjectCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())
	projectRes, err := projectClient.CreateProject(createProjectCtx, createProjectReq)
	require.NoError(t, err)
	projectID := projectRes.GetProjectId()

	firstUpdate := organizationv1.UpdateProjectLimitsRequest_builder{
		ProjectId:            projectID,
		DefaultMemoryLimitMi: proto.Int32(256),
	}.Build()
	firstUpdateCtx, firstUpdateCallInfo := connect.NewClientContext(context.Background())
	firstUpdateCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	firstUpdateCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())
	_, err = projectClient.UpdateProjectLimits(firstUpdateCtx, firstUpdate)
	require.NoError(t, err)

	secondUpdate := organizationv1.UpdateProjectLimitsRequest_builder{
		ProjectId:            projectID,
		DefaultMemoryLimitMi: proto.Int32(512),
	}.Build()
	secondUpdateCtx, secondUpdateCallInfo := connect.NewClientContext(context.Background())
	secondUpdateCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	secondUpdateCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())
	_, err = projectClient.UpdateProjectLimits(secondUpdateCtx, secondUpdate)
	require.NoError(t, err)

	getReq := organizationv1.GetProjectLimitsRequest_builder{ProjectId: projectID}.Build()
	getCtx, getCallInfo := connect.NewClientContext(context.Background())
	getCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	getCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())
	getRes, err := projectClient.GetProjectLimits(getCtx, getReq)
	require.NoError(t, err)

	assert.EqualValues(t, 512, getRes.GetLimits().GetDefaultMemoryLimitMi())
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

	createClusterReq := organizationv1.CreateClusterRequest_builder{
		Name: "test-cluster", Region: "eu-west-1", KubernetesVersion: "1.28",
	}.Build()
	createClusterCtx, createClusterCallInfo := connect.NewClientContext(context.Background())
	createClusterCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createClusterCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())
	clusterRes, err := clusterClient.CreateCluster(createClusterCtx, createClusterReq)
	require.NoError(t, err)
	clusterID := clusterRes.GetClusterId()

	createProject1Req := organizationv1.CreateProjectRequest_builder{
		ClusterId: clusterID, Name: "project-one",
	}.Build()
	createProject1Ctx, createProject1CallInfo := connect.NewClientContext(context.Background())
	createProject1CallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createProject1CallInfo.RequestHeader().Set("Fun-Organization", orgID.String())
	project1Res, err := projectClient.CreateProject(createProject1Ctx, createProject1Req)
	require.NoError(t, err)

	createProject2Req := organizationv1.CreateProjectRequest_builder{
		ClusterId: clusterID, Name: "project-two",
	}.Build()
	createProject2Ctx, createProject2CallInfo := connect.NewClientContext(context.Background())
	createProject2CallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createProject2CallInfo.RequestHeader().Set("Fun-Organization", orgID.String())
	project2Res, err := projectClient.CreateProject(createProject2Ctx, createProject2Req)
	require.NoError(t, err)

	updateReq := organizationv1.UpdateProjectLimitsRequest_builder{
		ProjectId:            project1Res.GetProjectId(),
		DefaultMemoryLimitMi: proto.Int32(256),
	}.Build()
	updateCtx, updateCallInfo := connect.NewClientContext(context.Background())
	updateCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	updateCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())
	_, err = projectClient.UpdateProjectLimits(updateCtx, updateReq)
	require.NoError(t, err)

	// project2 should see no limits
	getReq := organizationv1.GetProjectLimitsRequest_builder{
		ProjectId: project2Res.GetProjectId(),
	}.Build()
	getCtx, getCallInfo := connect.NewClientContext(context.Background())
	getCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	getCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())
	getRes, err := projectClient.GetProjectLimits(getCtx, getReq)
	require.NoError(t, err)

	assert.False(t, getRes.GetLimits().HasDefaultMemoryLimitMi())
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
	createClusterReq := organizationv1.CreateClusterRequest_builder{
		Name: "test-cluster", Region: "eu-west-1", KubernetesVersion: "1.28",
	}.Build()
	createClusterCtx, createClusterCallInfo := connect.NewClientContext(context.Background())
	createClusterCallInfo.RequestHeader().Set("Authorization", "Bearer "+token1)
	createClusterCallInfo.RequestHeader().Set("Fun-Organization", org1ID.String())
	clusterRes, err := clusterClient.CreateCluster(createClusterCtx, createClusterReq)
	require.NoError(t, err)

	createProjectReq := organizationv1.CreateProjectRequest_builder{
		ClusterId: clusterRes.GetClusterId(), Name: "test-project",
	}.Build()
	createProjectCtx, createProjectCallInfo := connect.NewClientContext(context.Background())
	createProjectCallInfo.RequestHeader().Set("Authorization", "Bearer "+token1)
	createProjectCallInfo.RequestHeader().Set("Fun-Organization", org1ID.String())
	projectRes, err := projectClient.CreateProject(createProjectCtx, createProjectReq)
	require.NoError(t, err)
	projectID := projectRes.GetProjectId()

	// Set limits on org1's project
	updateReq := organizationv1.UpdateProjectLimitsRequest_builder{
		ProjectId:            projectID,
		DefaultMemoryLimitMi: proto.Int32(256),
	}.Build()
	updateCtx, updateCallInfo := connect.NewClientContext(context.Background())
	updateCallInfo.RequestHeader().Set("Authorization", "Bearer "+token1)
	updateCallInfo.RequestHeader().Set("Fun-Organization", org1ID.String())
	_, err = projectClient.UpdateProjectLimits(updateCtx, updateReq)
	require.NoError(t, err)

	// user2 (org2) should not see org1's project limits
	getReq := organizationv1.GetProjectLimitsRequest_builder{ProjectId: projectID}.Build()
	getCtx, getCallInfo := connect.NewClientContext(context.Background())
	getCallInfo.RequestHeader().Set("Authorization", "Bearer "+token2)
	getCallInfo.RequestHeader().Set("Fun-Organization", org2ID.String())
	getRes, err := projectClient.GetProjectLimits(getCtx, getReq)
	require.NoError(t, err)
	assert.False(t, getRes.GetLimits().HasDefaultMemoryLimitMi())
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

	createClusterReq := organizationv1.CreateClusterRequest_builder{
		Name: "test-cluster", Region: "eu-west-1", KubernetesVersion: "1.28",
	}.Build()
	createClusterCtx, createClusterCallInfo := connect.NewClientContext(context.Background())
	createClusterCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createClusterCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())
	clusterRes, err := clusterClient.CreateCluster(createClusterCtx, createClusterReq)
	require.NoError(t, err)

	createProjectReq := organizationv1.CreateProjectRequest_builder{
		ClusterId: clusterRes.GetClusterId(), Name: "test-project",
	}.Build()
	createProjectCtx, createProjectCallInfo := connect.NewClientContext(context.Background())
	createProjectCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createProjectCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())
	projectRes, err := projectClient.CreateProject(createProjectCtx, createProjectReq)
	require.NoError(t, err)

	req := organizationv1.UpdateProjectLimitsRequest_builder{
		ProjectId:              projectRes.GetProjectId(),
		DefaultMemoryRequestMi: proto.Int32(256),
		DefaultMemoryLimitMi:   proto.Int32(128),
	}.Build()
	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	_, err = projectClient.UpdateProjectLimits(ctx, req)

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

	createClusterReq := organizationv1.CreateClusterRequest_builder{
		Name: "test-cluster", Region: "eu-west-1", KubernetesVersion: "1.28",
	}.Build()
	createClusterCtx, createClusterCallInfo := connect.NewClientContext(context.Background())
	createClusterCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createClusterCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())
	clusterRes, err := clusterClient.CreateCluster(createClusterCtx, createClusterReq)
	require.NoError(t, err)

	createProjectReq := organizationv1.CreateProjectRequest_builder{
		ClusterId: clusterRes.GetClusterId(), Name: "test-project",
	}.Build()
	createProjectCtx, createProjectCallInfo := connect.NewClientContext(context.Background())
	createProjectCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createProjectCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())
	projectRes, err := projectClient.CreateProject(createProjectCtx, createProjectReq)
	require.NoError(t, err)

	req := organizationv1.UpdateProjectLimitsRequest_builder{
		ProjectId:          projectRes.GetProjectId(),
		DefaultCpuRequestM: proto.Int32(500),
		DefaultCpuLimitM:   proto.Int32(100),
	}.Build()
	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	_, err = projectClient.UpdateProjectLimits(ctx, req)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}
