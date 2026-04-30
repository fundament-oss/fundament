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

	_, err := client.GetProjectLimits(context.Background(), connect.NewRequest(
		organizationv1.GetProjectLimitsRequest_builder{ProjectId: uuid.New().String()}.Build(),
	))

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

	getReq := connect.NewRequest(organizationv1.GetProjectLimitsRequest_builder{
		ProjectId: projectRes.Msg.GetProjectId(),
	}.Build())
	getReq.Header().Set("Authorization", "Bearer "+token)
	getReq.Header().Set("Fun-Organization", orgID.String())

	res, err := projectClient.GetProjectLimits(context.Background(), getReq)
	require.NoError(t, err)

	limits := res.Msg.GetLimits()
	require.NotNil(t, limits)
	assert.False(t, limits.HasDefaultMemoryRequestMi())
	assert.False(t, limits.HasDefaultMemoryLimitMi())
	assert.False(t, limits.HasDefaultCpuRequestM())
	assert.False(t, limits.HasDefaultCpuLimitM())
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

	res, err := projectClient.GetProjectLimits(context.Background(), getReq)
	require.NoError(t, err)

	limits := res.Msg.GetLimits()
	require.NotNil(t, limits)
	assert.EqualValues(t, 128, limits.GetDefaultMemoryRequestMi())
	assert.EqualValues(t, 256, limits.GetDefaultMemoryLimitMi())
	assert.EqualValues(t, 100, limits.GetDefaultCpuRequestM())
	assert.EqualValues(t, 500, limits.GetDefaultCpuLimitM())
}
