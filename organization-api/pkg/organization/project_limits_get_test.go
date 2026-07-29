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

func Test_ProjectLimits_Get_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)

	_, err := client.GetProjectLimits(context.Background(),
		organizationv1.GetProjectLimitsRequest_builder{ProjectId: uuid.New().String()}.Build(),
	)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_ProjectLimits_Get_NoLimitsSet(t *testing.T) {
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

	getReq := organizationv1.GetProjectLimitsRequest_builder{
		ProjectId: projectRes.GetProjectId(),
	}.Build()
	getCtx, getCallInfo := connect.NewClientContext(context.Background())
	getCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	getCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	res, err := projectClient.GetProjectLimits(getCtx, getReq)
	require.NoError(t, err)

	limits := res.GetLimits()
	require.NotNil(t, limits)
	assert.False(t, limits.HasDefaultMemoryRequestMi())
	assert.False(t, limits.HasDefaultMemoryLimitMi())
	assert.False(t, limits.HasDefaultCpuRequestM())
	assert.False(t, limits.HasDefaultCpuLimitM())

	// Platform defaults are always returned, even when the project has no limits set.
	defaults := res.GetDefaults()
	require.NotNil(t, defaults)
	assert.EqualValues(t, 256, defaults.GetDefaultMemoryRequestMi())
	assert.EqualValues(t, 512, defaults.GetDefaultMemoryLimitMi())
	assert.EqualValues(t, 100, defaults.GetDefaultCpuRequestM())
	assert.EqualValues(t, 500, defaults.GetDefaultCpuLimitM())
}

func Test_ProjectLimits_Get(t *testing.T) {
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

	res, err := projectClient.GetProjectLimits(getCtx, getReq)
	require.NoError(t, err)

	limits := res.GetLimits()
	require.NotNil(t, limits)
	assert.EqualValues(t, 128, limits.GetDefaultMemoryRequestMi())
	assert.EqualValues(t, 256, limits.GetDefaultMemoryLimitMi())
	assert.EqualValues(t, 100, limits.GetDefaultCpuRequestM())
	assert.EqualValues(t, 500, limits.GetDefaultCpuLimitM())
}
