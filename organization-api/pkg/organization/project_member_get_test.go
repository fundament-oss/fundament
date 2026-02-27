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

	_, err := client.GetProjectMember(context.Background(), connect.NewRequest(organizationv1.GetProjectMemberRequest_builder{}.Build()))

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
		Name:      "arbitrary",
	}.Build())
	createProjectReq.Header().Set("Authorization", "Bearer "+token)
	createProjectReq.Header().Set("Fun-Organization", orgID.String())

	createProjectRes, err := client.CreateProject(context.Background(), createProjectReq)
	require.NoError(t, err)

	addProjectMemberReq := connect.NewRequest(organizationv1.AddProjectMemberRequest_builder{
		ProjectId: createProjectRes.Msg.GetProjectId(),
		UserId:    projectMemberUserID.String(),
		Role:      organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER,
	}.Build())
	addProjectMemberReq.Header().Set("Authorization", "Bearer "+token)
	addProjectMemberReq.Header().Set("Fun-Organization", orgID.String())

	addMemberRes, err := client.AddProjectMember(context.Background(), addProjectMemberReq)
	require.NoError(t, err)

	tests := map[string]struct {
		Setup             func(*testing.T)
		Request           *organizationv1.GetProjectMemberRequest
		ExpectedErrorCode connect.Code
		ExpectedResponse  *organizationv1.GetProjectMemberResponse
	}{
		"non_existing_member_id": {
			Request: organizationv1.GetProjectMemberRequest_builder{
				MemberId: uuid.New().String(), // random new uuid
			}.Build(),
			ExpectedErrorCode: connect.CodeNotFound,
		},
		"happy_flow": {
			Request: organizationv1.GetProjectMemberRequest_builder{
				MemberId: addMemberRes.Msg.GetMemberId(),
			}.Build(),
			ExpectedResponse: organizationv1.GetProjectMemberResponse_builder{
				Member: organizationv1.ProjectMember_builder{
					Id:        addMemberRes.Msg.GetMemberId(),
					ProjectId: createProjectRes.Msg.GetProjectId(),
					UserId:    projectMemberUserID.String(),
					UserName:  "project-member-name",
					Role:      organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER,
				}.Build(),
			}.Build(),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			getProjectMemberReq := connect.NewRequest(tc.Request)
			getProjectMemberReq.Header().Set("Authorization", "Bearer "+token)
			getProjectMemberReq.Header().Set("Fun-Organization", orgID.String())

			res, err := client.GetProjectMember(context.Background(), getProjectMemberReq)

			if tc.ExpectedErrorCode != 0 {
				var connectErr *connect.Error
				require.ErrorAs(t, err, &connectErr)
				assert.Equal(t, tc.ExpectedErrorCode, connectErr.Code())
			} else {
				assert.True(t, res.Msg.HasMember())
				assert.Equal(t, tc.ExpectedResponse.GetMember().GetProjectId(), res.Msg.GetMember().GetProjectId())
				assert.Equal(t, tc.ExpectedResponse.GetMember().GetId(), res.Msg.GetMember().GetId())
				assert.Equal(t, tc.ExpectedResponse.GetMember().GetUserId(), res.Msg.GetMember().GetUserId())
				assert.Equal(t, tc.ExpectedResponse.GetMember().GetRole().String(), res.Msg.GetMember().GetRole().String())
				assert.Equal(t, tc.ExpectedResponse.GetMember().GetUserName(), res.Msg.GetMember().GetUserName())
			}
		})
	}
}
