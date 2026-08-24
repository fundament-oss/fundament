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

func Test_Project_Create_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)

	_, err := client.CreateProject(context.Background(), organizationv1.CreateProjectRequest_builder{
		ClusterId: uuid.New().String(),
		Name:      "test-project",
	}.Build())

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_Project_Create(t *testing.T) {
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
	createClusterReq := organizationv1.CreateClusterRequest_builder{
		Name:              "test-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build()
	createClusterCtx, createClusterCallInfo := connect.NewClientContext(context.Background())
	createClusterCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createClusterCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	createClusterRes, err := clusterClient.CreateCluster(createClusterCtx, createClusterReq)
	require.NoError(t, err)

	client := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)

	createReq := organizationv1.CreateProjectRequest_builder{
		ClusterId: createClusterRes.GetClusterId(),
		Name:      "test-project",
	}.Build()
	createCtx, createCallInfo := connect.NewClientContext(context.Background())
	createCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	res, err := client.CreateProject(createCtx, createReq)
	require.NoError(t, err)

	require.NotEmpty(t, res.GetProjectId())

	listMembersReq := organizationv1.ListProjectMembersRequest_builder{
		ProjectId: res.GetProjectId(),
	}.Build()
	listMembersCtx, listMembersCallInfo := connect.NewClientContext(context.Background())
	listMembersCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	listMembersCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	membersRes, err := client.ListProjectMembers(listMembersCtx, listMembersReq)
	require.NoError(t, err)

	members := membersRes.GetMembers()
	require.Len(t, members, 1)
	assert.Equal(t, userID.String(), members[0].GetUserId())
	assert.Equal(t, organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_ADMIN, members[0].GetRole())
}
