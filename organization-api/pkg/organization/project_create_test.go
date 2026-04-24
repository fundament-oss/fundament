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

	_, err := client.CreateProject(context.Background(), connect.NewRequest(organizationv1.CreateProjectRequest_builder{
		ClusterId: uuid.New().String(),
		Name:      "test-project",
	}.Build()))

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
	createClusterReq := connect.NewRequest(organizationv1.CreateClusterRequest_builder{
		Name:              "test-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build())
	createClusterReq.Header().Set("Authorization", "Bearer "+token)
	createClusterReq.Header().Set("Fun-Organization", orgID.String())

	createClusterRes, err := clusterClient.CreateCluster(context.Background(), createClusterReq)
	require.NoError(t, err)

	client := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)

	createReq := connect.NewRequest(organizationv1.CreateProjectRequest_builder{
		ClusterId: createClusterRes.Msg.GetClusterId(),
		Name:      "test-project",
	}.Build())
	createReq.Header().Set("Authorization", "Bearer "+token)
	createReq.Header().Set("Fun-Organization", orgID.String())

	res, err := client.CreateProject(context.Background(), createReq)
	require.NoError(t, err)

	require.NotEmpty(t, res.Msg.GetProjectId())

	listMembersReq := connect.NewRequest(organizationv1.ListProjectMembersRequest_builder{
		ProjectId: res.Msg.GetProjectId(),
	}.Build())
	listMembersReq.Header().Set("Authorization", "Bearer "+token)
	listMembersReq.Header().Set("Fun-Organization", orgID.String())

	membersRes, err := client.ListProjectMembers(context.Background(), listMembersReq)
	require.NoError(t, err)

	members := membersRes.Msg.GetMembers()
	require.Len(t, members, 1)
	assert.Equal(t, userID.String(), members[0].GetUserId())
	assert.Equal(t, organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_ADMIN, members[0].GetRole())
}
