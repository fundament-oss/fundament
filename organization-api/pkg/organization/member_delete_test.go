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

func Test_MemberDelete_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)

	client := organizationv1connect.NewMemberServiceClient(env.server.Client(), env.server.URL)

	_, err := client.DeleteMember(context.Background(), organizationv1.DeleteMemberRequest_builder{
		Id: "arbitrary",
	}.Build())

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_MemberDelete(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	callerUserID := uuid.New()
	targetUserID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     callerUserID,
			Name:   "caller-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithUser(&UserArgs{
			ID:     targetUserID,
			Name:   "target-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
	)

	token := env.createAuthnToken(t, callerUserID)

	client := organizationv1connect.NewMemberServiceClient(env.server.Client(), env.server.URL)

	// List members to discover the target member's membership ID
	theReq := &organizationv1.ListMembersRequest{}
	theCtx, theCallInfo := connect.NewClientContext(context.Background())
	theCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	theCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	listRes, err := client.ListMembers(theCtx, theReq)
	require.NoError(t, err)

	var targetMemberID string
	for _, m := range listRes.GetMembers() {
		if m.GetUserId() == targetUserID.String() {
			targetMemberID = m.GetId()
			break
		}
	}
	require.NotEmpty(t, targetMemberID, "target member not found in ListMembers response")

	tests := map[string]struct {
		Request  *organizationv1.DeleteMemberRequest
		WantCode connect.Code
		WantErr  bool
	}{
		"not_found": {
			Request: organizationv1.DeleteMemberRequest_builder{
				Id: uuid.New().String(),
			}.Build(),
			WantCode: connect.CodeNotFound,
			WantErr:  true,
		},
		"happy_flow": {
			Request: organizationv1.DeleteMemberRequest_builder{
				Id: targetMemberID,
			}.Build(),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			req := tc.Request
			ctx, callInfo := connect.NewClientContext(context.Background())
			callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
			callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

			res, err := client.DeleteMember(ctx, req)

			if tc.WantErr {
				var connectErr *connect.Error
				require.ErrorAs(t, err, &connectErr)
				assert.Equal(t, tc.WantCode, connectErr.Code())
				return
			}

			require.NoError(t, err)
			assert.Equal(t, "", res.String())

			getReq := organizationv1.GetMemberRequest_builder{
				UserId: new(targetUserID.String()),
			}.Build()
			getCtx, getCallInfo := connect.NewClientContext(context.Background())
			getCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
			getCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

			_, err = client.GetMember(getCtx, getReq)

			var connectErr *connect.Error
			require.ErrorAs(t, err, &connectErr)
			assert.Equal(t, connect.CodeNotFound, connectErr.Code())
		})
	}
}

// Removing someone from the organization removes them from its projects too,
// unless that would leave a project without an admin.
func Test_MemberDelete_RemovesProjectMemberships(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	callerUserID := uuid.New()
	targetUserID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     callerUserID,
			Name:   "caller-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithUser(&UserArgs{
			ID:     targetUserID,
			Name:   "target-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
	)

	token := env.createAuthnToken(t, callerUserID)
	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	memberClient := organizationv1connect.NewMemberServiceClient(env.server.Client(), env.server.URL)
	clusterClient := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)
	projectClient := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)

	clusterRes, err := clusterClient.CreateCluster(ctx, organizationv1.CreateClusterRequest_builder{
		Name:              "test-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build())
	require.NoError(t, err)

	createProject := func(name string) string {
		res, err := projectClient.CreateProject(ctx, organizationv1.CreateProjectRequest_builder{
			ClusterId: clusterRes.GetClusterId(),
			Name:      name,
		}.Build())
		require.NoError(t, err)
		return res.GetProjectId()
	}
	addMember := func(projectID string, userID uuid.UUID, role organizationv1.ProjectMemberRole) string {
		res, err := projectClient.AddProjectMember(ctx, organizationv1.AddProjectMemberRequest_builder{
			ProjectId: projectID,
			UserId:    userID.String(),
			Role:      role,
		}.Build())
		require.NoError(t, err)
		return res.GetMemberId()
	}
	projectUserIDs := func(projectID string) []string {
		res, err := projectClient.ListProjectMembers(ctx, organizationv1.ListProjectMembersRequest_builder{
			ProjectId: projectID,
		}.Build())
		require.NoError(t, err)
		ids := make([]string, 0, len(res.GetMembers()))
		for _, m := range res.GetMembers() {
			ids = append(ids, m.GetUserId())
		}
		return ids
	}

	// team-a: the target is a viewer.
	teamA := createProject("team-a")
	addMember(teamA, targetUserID, organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER)

	makeTargetOnlyAdmin := func(projectID string) {
		addMember(projectID, targetUserID, organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_ADMIN)
		_, err := projectClient.RemoveProjectMember(ctx, organizationv1.RemoveProjectMemberRequest_builder{
			MemberId: projectMemberID(ctx, t, projectClient, projectID, callerUserID),
		}.Build())
		require.NoError(t, err)
	}

	// team-b: the target is the only admin.
	teamB := createProject("team-b")
	makeTargetOnlyAdmin(teamB)

	// team-c: the target is the only admin of a deleted project, which
	// neither blocks the removal nor shows up in the refusal.
	teamC := createProject("team-c")
	makeTargetOnlyAdmin(teamC)
	_, err = projectClient.DeleteProject(ctx, organizationv1.DeleteProjectRequest_builder{
		ProjectId: teamC,
	}.Build())
	require.NoError(t, err)

	memberRes, err := memberClient.GetMember(ctx, organizationv1.GetMemberRequest_builder{
		UserId: new(targetUserID.String()),
	}.Build())
	require.NoError(t, err)
	deleteReq := organizationv1.DeleteMemberRequest_builder{
		Id: memberRes.GetMember().GetId(),
	}.Build()

	// Refused as a whole while it would leave team-b without an admin.
	_, err = memberClient.DeleteMember(ctx, deleteReq)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
	assert.Contains(t, connectErr.Message(), `"team-b" on cluster "test-cluster"`)
	assert.NotContains(t, connectErr.Message(), `"team-a"`)
	assert.NotContains(t, connectErr.Message(), `"team-c"`)
	assert.Contains(t, projectUserIDs(teamA), targetUserID.String(), "nothing removed on refusal")

	_, err = memberClient.GetMember(ctx, organizationv1.GetMemberRequest_builder{
		UserId: new(targetUserID.String()),
	}.Build())
	require.NoError(t, err, "still an organization member on refusal")

	// With another admin in team-b, the removal goes through, and takes the
	// project memberships with it.
	addMember(teamB, callerUserID, organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_ADMIN)

	_, err = memberClient.DeleteMember(ctx, deleteReq)
	require.NoError(t, err)

	assert.NotContains(t, projectUserIDs(teamA), targetUserID.String())
	assert.NotContains(t, projectUserIDs(teamB), targetUserID.String())
	assert.Contains(t, projectUserIDs(teamA), callerUserID.String(), "other members are untouched")

	_, err = memberClient.DeleteMember(ctx, deleteReq)
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code(), "a removed membership is not found")
}

// The database removes the project memberships, so revoking a membership
// without the API, as funops does, removes them too.
func Test_MemberRevokedInDatabase_RemovesProjectMemberships(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	callerUserID := uuid.New()
	targetUserID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     callerUserID,
			Name:   "caller-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithUser(&UserArgs{
			ID:     targetUserID,
			Name:   "target-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
	)

	token := env.createAuthnToken(t, callerUserID)
	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	clusterClient := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)
	projectClient := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)

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

	_, err = projectClient.AddProjectMember(ctx, organizationv1.AddProjectMemberRequest_builder{
		ProjectId: projectRes.GetProjectId(),
		UserId:    targetUserID.String(),
		Role:      organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER,
	}.Build())
	require.NoError(t, err)

	// The update funops' MembershipDelete makes.
	_, err = env.adminPool.Exec(t.Context(), `
		UPDATE tenant.organizations_users
		SET deleted = now(), status = 'revoked'
		WHERE organization_id = $1 AND user_id = $2 AND deleted IS NULL`,
		orgID, targetUserID,
	)
	require.NoError(t, err)

	res, err := projectClient.ListProjectMembers(ctx, organizationv1.ListProjectMembersRequest_builder{
		ProjectId: projectRes.GetProjectId(),
	}.Build())
	require.NoError(t, err)
	userIDs := make([]string, 0, len(res.GetMembers()))
	for _, m := range res.GetMembers() {
		userIDs = append(userIDs, m.GetUserId())
	}
	assert.NotContains(t, userIDs, targetUserID.String())
	assert.Contains(t, userIDs, callerUserID.String())
}

func projectMemberID(
	ctx context.Context,
	t *testing.T,
	client organizationv1connect.ProjectServiceClient,
	projectID string,
	userID uuid.UUID,
) string {
	t.Helper()

	res, err := client.ListProjectMembers(ctx, organizationv1.ListProjectMembersRequest_builder{
		ProjectId: projectID,
	}.Build())
	require.NoError(t, err)

	for _, m := range res.GetMembers() {
		if m.GetUserId() == userID.String() {
			return m.GetId()
		}
	}
	require.FailNow(t, "user is not a member of the project", "user_id %s", userID)
	return ""
}
