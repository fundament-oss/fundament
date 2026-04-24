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

func Test_ProjectMember_Add_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)

	_, err := client.AddProjectMember(context.Background(), connect.NewRequest(organizationv1.AddProjectMemberRequest_builder{
		ProjectId: uuid.New().String(),
		UserId:    uuid.New().String(),
		Role:      organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER,
	}.Build()))

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_ProjectMember_Add(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	callerUserID := uuid.New()
	newMemberUserID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     callerUserID,
			Name:   "caller-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithUser(&UserArgs{
			ID:     newMemberUserID,
			Name:   "new-member",
			OrgIDs: []uuid.UUID{orgID},
		}),
	)

	token := env.createAuthnToken(t, callerUserID)

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

	createProjectReq := connect.NewRequest(organizationv1.CreateProjectRequest_builder{
		ClusterId: createClusterRes.Msg.GetClusterId(),
		Name:      "test-project",
	}.Build())
	createProjectReq.Header().Set("Authorization", "Bearer "+token)
	createProjectReq.Header().Set("Fun-Organization", orgID.String())

	createProjectRes, err := client.CreateProject(context.Background(), createProjectReq)
	require.NoError(t, err)

	projectID := createProjectRes.Msg.GetProjectId()

	addReq := connect.NewRequest(organizationv1.AddProjectMemberRequest_builder{
		ProjectId: projectID,
		UserId:    newMemberUserID.String(),
		Role:      organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER,
	}.Build())
	addReq.Header().Set("Authorization", "Bearer "+token)
	addReq.Header().Set("Fun-Organization", orgID.String())

	res, err := client.AddProjectMember(context.Background(), addReq)
	require.NoError(t, err)
	assert.NotEmpty(t, res.Msg.GetMemberId())

	// Adding the same user a second time must fail.
	_, err = client.AddProjectMember(context.Background(), addReq)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeAlreadyExists, connectErr.Code())

	invalidRoleReq := connect.NewRequest(organizationv1.AddProjectMemberRequest_builder{
		ProjectId: projectID,
		UserId:    uuid.New().String(),
		Role:      organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_UNSPECIFIED,
	}.Build())
	invalidRoleReq.Header().Set("Authorization", "Bearer "+token)
	invalidRoleReq.Header().Set("Fun-Organization", orgID.String())

	_, err = client.AddProjectMember(context.Background(), invalidRoleReq)
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}
