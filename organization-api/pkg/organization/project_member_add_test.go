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

	_, err := client.AddProjectMember(context.Background(), organizationv1.AddProjectMemberRequest_builder{
		ProjectId: uuid.New().String(),
		UserId:    uuid.New().String(),
		Role:      organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER,
	}.Build())

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
		Name:      "test-project",
	}.Build()
	createProjectCtx, createProjectCallInfo := connect.NewClientContext(context.Background())
	createProjectCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createProjectCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	createProjectRes, err := client.CreateProject(createProjectCtx, createProjectReq)
	require.NoError(t, err)

	projectID := createProjectRes.GetProjectId()

	addReq := organizationv1.AddProjectMemberRequest_builder{
		ProjectId: projectID,
		UserId:    newMemberUserID.String(),
		Role:      organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER,
	}.Build()
	addCtx, addCallInfo := connect.NewClientContext(context.Background())
	addCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	addCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	res, err := client.AddProjectMember(addCtx, addReq)
	require.NoError(t, err)
	assert.NotEmpty(t, res.GetMemberId())

	// Adding the same user a second time must fail.
	_, err = client.AddProjectMember(addCtx, addReq)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeAlreadyExists, connectErr.Code())

	invalidRoleReq := organizationv1.AddProjectMemberRequest_builder{
		ProjectId: projectID,
		UserId:    uuid.New().String(),
		Role:      organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_UNSPECIFIED,
	}.Build()
	invalidRoleCtx, invalidRoleCallInfo := connect.NewClientContext(context.Background())
	invalidRoleCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	invalidRoleCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	_, err = client.AddProjectMember(invalidRoleCtx, invalidRoleReq)
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// Only accepted organization members can be added to a project: a membership
// for anyone else grants nothing, and would come back into effect when they
// join the organization later.
func Test_ProjectMember_Add_RequiresAcceptedOrgMember(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	callerUserID := uuid.New()
	inviteeUserID := uuid.New()
	outsiderUserID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     callerUserID,
			Name:   "caller-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithUser(&UserArgs{
			ID:    inviteeUserID,
			Name:  "invitee",
			Email: "invitee@example.com",
		}),
		WithUser(&UserArgs{
			ID:   outsiderUserID,
			Name: "outsider",
		}),
	)

	token := env.createAuthnToken(t, callerUserID)
	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	inviteClient := organizationv1connect.NewInviteServiceClient(env.server.Client(), env.server.URL)
	clusterClient := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)
	projectClient := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)

	_, err := inviteClient.InviteMember(ctx, organizationv1.InviteMemberRequest_builder{
		Email:      "invitee@example.com",
		Permission: "viewer",
	}.Build())
	require.NoError(t, err)

	clusterRes, err := clusterClient.CreateCluster(ctx, organizationv1.CreateClusterRequest_builder{
		Name:              "test-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build())
	require.NoError(t, err)

	projectRes, err := projectClient.CreateProject(ctx, organizationv1.CreateProjectRequest_builder{
		ClusterId: clusterRes.GetClusterId(),
		Name:      "test-project",
	}.Build())
	require.NoError(t, err)

	tests := map[string]uuid.UUID{
		"pending invitee":         inviteeUserID,
		"not in the organization": outsiderUserID,
	}

	for name, userID := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := projectClient.AddProjectMember(ctx, organizationv1.AddProjectMemberRequest_builder{
				ProjectId: projectRes.GetProjectId(),
				UserId:    userID.String(),
				Role:      organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER,
			}.Build())

			var connectErr *connect.Error
			require.ErrorAs(t, err, &connectErr)
			assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
		})
	}
}
