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

func Test_ProjectMember_Get_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)

	client := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)

	_, err := client.GetProjectMember(context.Background(), connect.NewRequest(&organizationv1.GetProjectMemberRequest{}))

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_ProjectMember_Get(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()
	projectMemberUserID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(userID, "test-user", []uuid.UUID{orgID}),
		WithUser(projectMemberUserID, "project-member-name", []uuid.UUID{orgID}),
	)

	token := env.createAuthnToken(t, userID)

	client := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)

	createProjectReq := connect.NewRequest(&organizationv1.CreateProjectRequest{
		Name: "arbitrary",
	})
	createProjectReq.Header().Set("Authorization", "Bearer "+token)
	createProjectReq.Header().Set("Fun-Organization", orgID.String())

	createProjectRes, err := client.CreateProject(context.Background(), createProjectReq)
	require.NoError(t, err)

	addProjectMemberReq := connect.NewRequest(&organizationv1.AddProjectMemberRequest{
		ProjectId: createProjectRes.Msg.ProjectId,
		UserId:    projectMemberUserID.String(),
		Role:      organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER,
	})
	addProjectMemberReq.Header().Set("Authorization", "Bearer "+token)
	addProjectMemberReq.Header().Set("Fun-Organization", orgID.String())

	addMemberRes, err := client.AddProjectMember(context.Background(), addProjectMemberReq)
	require.NoError(t, err)

	getProjectMemberReq := connect.NewRequest(&organizationv1.GetProjectMemberRequest{
		MemberId: addMemberRes.Msg.MemberId,
	})
	getProjectMemberReq.Header().Set("Authorization", "Bearer "+token)
	getProjectMemberReq.Header().Set("Fun-Organization", orgID.String())

	res, err := client.GetProjectMember(context.Background(), getProjectMemberReq)
	require.NoError(t, err)

	assert.NotNil(t, res.Msg.Member)
	assert.Equal(t, createProjectRes.Msg.ProjectId, res.Msg.Member.ProjectId)
	assert.Equal(t, addMemberRes.Msg.MemberId, res.Msg.Member.Id)
	assert.Equal(t, projectMemberUserID.String(), res.Msg.Member.UserId)
	assert.Equal(t, organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER, res.Msg.Member.Role)
	assert.Equal(t, "project-member-name", res.Msg.Member.UserName)
}
