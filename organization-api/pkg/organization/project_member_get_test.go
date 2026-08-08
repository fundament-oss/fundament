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

	_, err := client.GetProjectMember(context.Background(), organizationv1.GetProjectMemberRequest_builder{}.Build())

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
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithUser(&UserArgs{
			ID:     projectMemberUserID,
			Name:   "project-member-name",
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

	createProjectReq := organizationv1.CreateProjectRequest_builder{
		ClusterId: createClusterRes.GetClusterId(),
		Name:      "arbitrary",
	}.Build()
	createProjectCtx, createProjectCallInfo := connect.NewClientContext(context.Background())
	createProjectCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createProjectCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	createProjectRes, err := client.CreateProject(createProjectCtx, createProjectReq)
	require.NoError(t, err)

	addProjectMemberReq := organizationv1.AddProjectMemberRequest_builder{
		ProjectId: createProjectRes.GetProjectId(),
		UserId:    projectMemberUserID.String(),
		Role:      organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER,
	}.Build()
	addProjectMemberCtx, addProjectMemberCallInfo := connect.NewClientContext(context.Background())
	addProjectMemberCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	addProjectMemberCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	addMemberRes, err := client.AddProjectMember(addProjectMemberCtx, addProjectMemberReq)
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
				MemberId: addMemberRes.GetMemberId(),
			}.Build(),
			ExpectedResponse: organizationv1.GetProjectMemberResponse_builder{
				Member: organizationv1.ProjectMember_builder{
					Id:        addMemberRes.GetMemberId(),
					ProjectId: createProjectRes.GetProjectId(),
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

			getProjectMemberReq := tc.Request
			getProjectMemberCtx, getProjectMemberCallInfo := connect.NewClientContext(context.Background())
			getProjectMemberCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
			getProjectMemberCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

			res, err := client.GetProjectMember(getProjectMemberCtx, getProjectMemberReq)

			if tc.ExpectedErrorCode != 0 {
				var connectErr *connect.Error
				require.ErrorAs(t, err, &connectErr)
				assert.Equal(t, tc.ExpectedErrorCode, connectErr.Code())
			} else {
				assert.True(t, res.HasMember())
				assert.Equal(t, tc.ExpectedResponse.GetMember().GetProjectId(), res.GetMember().GetProjectId())
				assert.Equal(t, tc.ExpectedResponse.GetMember().GetId(), res.GetMember().GetId())
				assert.Equal(t, tc.ExpectedResponse.GetMember().GetUserId(), res.GetMember().GetUserId())
				assert.Equal(t, tc.ExpectedResponse.GetMember().GetRole().String(), res.GetMember().GetRole().String())
				assert.Equal(t, tc.ExpectedResponse.GetMember().GetUserName(), res.GetMember().GetUserName())
			}
		})
	}
}
